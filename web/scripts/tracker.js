// Builds a tracker page (history charts with a time-range picker) from a
// config, using SC from sc.js. The history comes from the controller's
// /realtime/get... endpoints; while the range ends at "now", new points are
// added live from the /realtime/statusEvents stream.
//
// SC.tracker({
//   title, sub,
//   ranges: [minutes, ...], defaultRange: minutes,
//   selector: {label, options: [{value, label}], value},   // optional, e.g. solar power/volts/amps
//   sources: {name: {url, step, map: (row) => ({...})}},    // map: one JSON row to chart fields
//   charts: [{source, title, height, unit (or (selectorValue) => unit), decimals, zero, min, max, minSpan, step, gapMs,
//             series: [{label, color, dash, key}] | (selectorValue) => [...]}],
//   live: (statusMessage) => ({sourceName: {t, ...fields}}),  // optional
// })
(function () {
  "use strict";
  const { $, h, LineChart } = SC;
  const MAX_POINTS = 1200;   // more rows than this are averaged into buckets
  const RANGE_LABELS = { 15: "15 min", 30: "30 min", 60: "1 h", 120: "2 h", 240: "4 h", 480: "8 h", 720: "12 h",
                         1440: "24 h", 2880: "2 days", 4320: "3 days", 10080: "1 week" };

  // The controller expects UTC times as "2006-1-2 15:4".
  function apiTime(t) {
    const d = new Date(t);
    return d.getUTCFullYear() + "-" + (d.getUTCMonth() + 1) + "-" + d.getUTCDate() + " " + d.getUTCHours() + ":" + d.getUTCMinutes();
  }
  function localInput(t) {
    const d = new Date(t), p = (n) => String(n).padStart(2, "0");
    return d.getFullYear() + "-" + p(d.getMonth() + 1) + "-" + p(d.getDate()) + "T" + p(d.getHours()) + ":" + p(d.getMinutes());
  }

  // Averages rows into buckets so long ranges stay quick to draw. Step data
  // keeps the last value in each bucket instead of the mean.
  function thin(rows, t0, t1, step) {
    if (rows.length <= MAX_POINTS) return rows;
    const width = (t1 - t0) / (MAX_POINTS / 1.2);
    const out = [];
    let cur = null, key = null, n = 0;
    const flush = () => {
      if (!cur) return;
      if (!step) for (const k in cur) if (k !== "t" && typeof cur[k] === "number") cur[k] /= n;
      out.push(cur);
    };
    for (const r of rows) {
      const b = Math.floor((r.t - t0) / width);
      if (b !== key) {
        flush();
        key = b; n = 1;
        cur = Object.assign({}, r);
        continue;
      }
      n++;
      if (step) { const t = cur.t; cur = Object.assign({}, r); cur.t = t; }
      else for (const k in r) if (k !== "t" && typeof r[k] === "number") cur[k] += r[k];
    }
    flush();
    return out;
  }

  SC.tracker = function (cfg) {
    const key = "tracker-" + location.pathname;
    let saved = {};
    try { saved = JSON.parse(localStorage.getItem(key)) || {}; } catch (e) { /* storage unavailable */ }
    const save = () => { try { localStorage.setItem(key, JSON.stringify({ range: rangeMin, sel: selValue })); } catch (e) { /* ignore */ } };

    const ranges = cfg.ranges || [30, 60, 120, 240, 480, 720, 1440, 2880, 4320];
    let rangeMin = ranges.includes(saved.range) ? saved.range : (cfg.defaultRange || 480);
    let custom = null;   // {t0, t1} when a custom range is shown
    let selValue = cfg.selector ? (cfg.selector.options.some((o) => o.value === saved.sel) ? saved.sel : cfg.selector.value) : null;

    document.title = cfg.title;
    $("title").textContent = cfg.title;
    if (cfg.sub) $("sub").textContent = cfg.sub;
    const statusBox = $("status");

    // ----- controls -----
    const controls = $("controls");
    const seg = h("div", { class: "seg", role: "group", "aria-label": "Time range" });
    const buttons = {};
    for (const r of ranges) {
      const b = h("button", { type: "button" }, RANGE_LABELS[r] || r + " min");
      b.addEventListener("click", () => { rangeMin = r; custom = null; save(); syncButtons(); load(); });
      buttons[r] = b; seg.appendChild(b);
    }
    const customBtn = h("button", { type: "button" }, "Custom");
    seg.appendChild(customBtn);
    controls.appendChild(seg);

    const customBox = h("span", { class: "controls", style: "margin:0; display:none" });
    const fromIn = h("input", { type: "datetime-local", "aria-label": "From" });
    const toIn = h("input", { type: "datetime-local", "aria-label": "To" });
    const showBtn = h("button", { type: "button", class: "btn" }, "Show");
    customBox.append(h("label", {}, "From"), fromIn, h("label", {}, "to"), toIn, showBtn);
    controls.appendChild(customBox);
    customBtn.addEventListener("click", () => {
      const t1 = custom ? custom.t1 : Date.now(), t0 = custom ? custom.t0 : t1 - rangeMin * 60000;
      fromIn.value = localInput(t0); toIn.value = localInput(t1);
      customBox.style.display = "";
      customBtn.setAttribute("aria-pressed", "true");
      for (const r in buttons) buttons[r].setAttribute("aria-pressed", "false");
    });
    showBtn.addEventListener("click", () => {
      const t0 = new Date(fromIn.value).getTime(), t1 = new Date(toIn.value).getTime();
      if (!(t1 > t0)) { SC.setStatus(statusBox, "warn", "The end must be after the start"); return; }
      custom = { t0, t1 };
      load();
    });

    if (cfg.selector) {
      const sel = h("select", { "aria-label": cfg.selector.label });
      for (const o of cfg.selector.options) {
        const opt = h("option", { value: o.value }, o.label);
        if (o.value === selValue) opt.selected = true;
        sel.appendChild(opt);
      }
      sel.addEventListener("change", () => { selValue = sel.value; save(); render(); });
      controls.append(h("label", {}, cfg.selector.label), sel);
    }

    const csvBtn = h("button", { type: "button", class: "btn" }, "Download CSV");
    csvBtn.addEventListener("click", downloadCSV);
    controls.appendChild(csvBtn);

    function syncButtons() {
      for (const r in buttons) buttons[r].setAttribute("aria-pressed", String(!custom && +r === rangeMin));
      customBtn.setAttribute("aria-pressed", String(!!custom));
      customBox.style.display = custom ? "" : "none";
    }
    syncButtons();

    // ----- charts -----
    const box = $("charts");
    const charts = cfg.charts.map((c) => {
      const holder = h("div", { class: "chart" });
      box.appendChild(holder);
      const chart = new LineChart(holder, {
        height: c.height || 260, unit: typeof c.unit === "function" ? "" : c.unit, decimals: c.decimals == null ? 1 : c.decimals,
        zero: c.zero, min: c.min, max: c.max, minSpan: c.minSpan, step: c.step || cfg.sources[c.source].step,
        gapMs: c.gapMs, title: c.title, aria: c.title,
        series: () => (typeof c.series === "function" ? c.series(selValue) : c.series)
          .map((s) => ({ label: s.label, color: s.color, dash: s.dash, get: (row) => row[s.key] })),
      });
      return { cfg: c, chart };
    });

    // ----- data -----
    const data = {};   // source name -> rows sorted by t
    let loading = 0;   // increments so a slow, superseded request is ignored

    function window_() {
      if (custom) return custom;
      const t1 = Date.now();
      return { t0: t1 - rangeMin * 60000, t1 };
    }

    function render() {
      const { t0, t1 } = window_();
      for (const { cfg: c, chart } of charts) {
        const src = cfg.sources[c.source];
        const rows = (data[c.source] || []).filter((r) => r.t >= t0 && r.t <= t1);
        if (typeof c.unit === "function") chart.o.unit = c.unit(selValue);
        chart.o.holdTo = custom ? Math.min(t1, Date.now()) : t1;
        chart.render(thin(rows, t0, t1, src.step), t0, t1);
      }
    }

    function load() {
      const id = ++loading;
      const { t0, t1 } = window_();
      for (const { chart } of charts) chart.emptyText = "Loading…";
      render();
      const q = "?start=" + encodeURIComponent(apiTime(t0)) + "&end=" + encodeURIComponent(apiTime(t1 + 60000));
      const names = Object.keys(cfg.sources);
      Promise.all(names.map((n) => fetch(cfg.sources[n].url + q, { cache: "no-store" })
        .then((r) => r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t || r.status))))))
        .then((results) => {
          if (id !== loading) return;
          names.forEach((n, i) => {
            const src = cfg.sources[n];
            data[n] = (results[i] || []).map((row) => {
              const out = src.map(row);
              out.t = parseFloat(row.logged) * 1000;
              return out;
            }).filter((r) => !isNaN(r.t)).sort((a, b) => a.t - b.t);
          });
          for (const { chart } of charts) chart.emptyText = null;
          if (custom) SC.setStatus(statusBox, "ok", "History");
          render();
        })
        .catch((err) => {
          if (id !== loading) return;
          for (const { chart } of charts) chart.emptyText = "Could not load the history";
          SC.setStatus(statusBox, "error", "Could not load the history: " + err.message);
          render();
        });
    }

    // ----- live -----
    if (cfg.live) {
      SC.stream({
        url: "/realtime/statusEvents",
        pollUrl: "/realtime/getStatus",
        onState: (kind, text) => { if (!custom) SC.setStatus(statusBox, kind, text); },
        onReconnect: () => { if (!custom) load(); },
        onMessage: (msg) => {
          if (custom || loading === 0) return;
          const add = cfg.live(msg) || {};
          const minGap = Math.max(1000, rangeMin * 60000 / MAX_POINTS);
          for (const n in add) {
            const row = add[n], rows = data[n];
            if (!row || !rows || !SC.isNum(row.t)) continue;
            const last = rows.length ? rows[rows.length - 1].t : 0;
            if (row.t - last >= minGap) rows.push(row);
          }
          const t0 = Date.now() - rangeMin * 60000 - 60000;
          for (const n in data) while (data[n].length > 2 && data[n][1].t < t0) data[n].shift();
          render();
        },
      });
    } else {
      SC.setStatus(statusBox, "ok", "History");
      setInterval(() => { if (!custom) load(); }, 60000);
    }

    let resizeTimer;
    window.addEventListener("resize", () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(render, 150); });
    load();

    function downloadCSV() {
      const { t0, t1 } = window_();
      const lines = [];
      for (const n in data) {
        const rows = data[n].filter((r) => r.t >= t0 && r.t <= t1);
        if (!rows.length) continue;
        const keys = Object.keys(rows[0]).filter((k) => k !== "t");
        lines.push(["time"].concat(keys).join(","));
        for (const r of rows) lines.push([new Date(r.t).toISOString()].concat(keys.map((k) => r[k])).join(","));
        lines.push("");
      }
      const a = h("a", { href: URL.createObjectURL(new Blob([lines.join("\n")], { type: "text/csv" })),
                         download: cfg.title.replace(/\W+/g, "_") + ".csv" });
      document.body.appendChild(a); a.click(); a.remove();
    }
  };
})();
