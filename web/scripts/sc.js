// Shared code for the SystemController pages: small DOM helpers, a line chart
// with hover tooltips, and a connector for the controller's Server-Sent Events
// streams. Exposes one global, SC.
(function () {
  "use strict";
  const SVGNS = "http://www.w3.org/2000/svg";

  // ---------- helpers ----------
  const $ = (id) => document.getElementById(id);
  const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  function el(tag, attrs, parent) {
    const e = document.createElementNS(SVGNS, tag);
    for (const k in attrs || {}) e.setAttribute(k, attrs[k]);
    if (parent) parent.appendChild(e);
    return e;
  }
  function h(tag, attrs, text) {
    const e = document.createElement(tag);
    for (const k in attrs || {}) e.setAttribute(k, attrs[k]);
    if (text != null) e.textContent = text;
    return e;
  }
  const isNum = (v) => v != null && !isNaN(v);
  const fmt = (v, d, unit) => !isNum(v) ? "–" : Number(v).toFixed(d) + (unit ? " " + unit : "");
  const fmtW = (w) => {
    if (!isNum(w)) return "–";
    const a = Math.abs(w);
    return a >= 1000 ? (w / 1000).toFixed(a >= 10000 ? 1 : 2) + " kW" : Math.round(w) + " W";
  };
  const pad2 = (n) => String(n).padStart(2, "0");
  const clock = (t) => { const d = new Date(t); return pad2(d.getHours()) + ":" + pad2(d.getMinutes()); };
  const clockS = (t) => { const d = new Date(t); return clock(t) + ":" + pad2(d.getSeconds()); };
  const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  const dayLabel = (t) => { const d = new Date(t); return DAYS[d.getDay()] + " " + d.getDate(); };
  const DASHES = { solid: "", dash: "6 4", dot: "1.5 3.5" };

  // A short line in the series colour (and dash), for legends and tooltips.
  function swatch(color, dash) {
    const s = el("svg", { width: 18, height: 6, "aria-hidden": "true" });
    el("line", { x1: 1, x2: 17, y1: 3, y2: 3, stroke: css(color), "stroke-width": 2, "stroke-linecap": "round", "stroke-dasharray": DASHES[dash || "solid"] }, s);
    return s;
  }

  // Status icons: ok (filled dot), warn (triangle), error (cross in circle).
  function icon(kind) {
    const s = el("svg", { width: 14, height: 14, viewBox: "0 0 14 14", "aria-hidden": "true" });
    if (kind === "ok") el("circle", { cx: 7, cy: 7, r: 5, fill: css("--good") }, s);
    else if (kind === "warn") {
      el("path", { d: "M7 1.5 L13 12.5 H1 Z", fill: css("--warning") }, s);
      el("path", { d: "M7 5.5 V8.6 M7 10.4 V10.6", stroke: "#0b0b0b", "stroke-width": 1.6, "stroke-linecap": "round" }, s);
    } else {
      el("circle", { cx: 7, cy: 7, r: 6, fill: css("--critical") }, s);
      el("path", { d: "M4.6 4.6 L9.4 9.4 M9.4 4.6 L4.6 9.4", stroke: "#fff", "stroke-width": 1.6, "stroke-linecap": "round" }, s);
    }
    return s;
  }

  function setStatus(box, kind, text) {
    box.textContent = "";
    box.appendChild(icon(kind));
    box.appendChild(document.createTextNode(text));
  }

  // ---------- line chart ----------
  function niceStep(span, count) {
    const raw = span / Math.max(count, 1);
    const mag = Math.pow(10, Math.floor(Math.log10(raw)));
    const n = raw / mag;
    return (n <= 1 ? 1 : n <= 2 ? 2 : n <= 2.5 ? 2.5 : n <= 5 ? 5 : 10) * mag;
  }
  const X_STEPS_MIN = [1, 2, 5, 10, 15, 30, 60, 120, 180, 360, 720, 1440];

  // LineChart draws time series into container. opts:
  //   height, unit, decimals, zero (include 0), min, max (always include),
  //   minSpan (smallest y range), title (shows the first series' latest value),
  //   step (draw as steps, holding each value until the next),
  //   gapMs (break the line where samples are further apart than this),
  //   aria, series: [{label, color: "--series-n", dash: "dash"|"dot", get: (row) => value}]
  //   or a function returning that list.
  // Rows are {t: ms, ...}; render(rows, t0, t1) draws the window t0..t1.
  function LineChart(container, opts) {
    this.c = container;
    this.o = opts;
    this.tip = h("div", { class: "tip" });
    if (opts.title) { this.titleEl = h("p", { class: "mini-title" }); container.appendChild(this.titleEl); }
    this.legend = h("div", { class: "legend" });
    container.appendChild(this.legend);
    this.svg = el("svg", { class: "plot", tabindex: "0", role: "img", "aria-label": opts.aria || opts.title || "" }, container);
    container.appendChild(this.tip);
    this.hoverIdx = null;
    this.svg.addEventListener("pointermove", (e) => this.onMove(e));
    this.svg.addEventListener("pointerleave", () => this.hide());
    this.svg.addEventListener("blur", () => this.hide());
    this.svg.addEventListener("keydown", (e) => this.onKey(e));
  }

  LineChart.prototype.seriesList = function () {
    return typeof this.o.series === "function" ? this.o.series() : this.o.series;
  };

  LineChart.prototype.drawLegend = function (series) {
    const key = series.map((s) => s.label + s.color + (s.dash || "")).join("|");
    if (key === this.legendKey) return;
    this.legendKey = key;
    this.legend.textContent = "";
    this.legend.style.display = series.length > 1 ? "" : "none";
    for (const se of series) {
      const s = h("span");
      s.appendChild(swatch(se.color, se.dash));
      s.appendChild(document.createTextNode(se.label));
      this.legend.appendChild(s);
    }
  };

  LineChart.prototype.render = function (data, t0, t1) {
    const o = this.o, svg = this.svg;
    const W = Math.max(this.c.clientWidth, 280), H = o.height || 240;
    const m = { l: 52, r: 14, t: 8, b: 22 };
    const pw = W - m.l - m.r, ph = H - m.t - m.b;
    svg.setAttribute("viewBox", "0 0 " + W + " " + H);
    svg.setAttribute("height", H);
    while (svg.firstChild) svg.removeChild(svg.firstChild);

    const series = this.seriesList();
    this.drawLegend(series);
    this.data = data; this.series = series;
    this.geom = { W, H, m, pw, ph, t0, t1 };

    if (this.titleEl) {
      const last = data.length ? series[0].get(data[data.length - 1]) : null;
      this.titleEl.textContent = "";
      this.titleEl.appendChild(document.createTextNode(o.title + " "));
      this.titleEl.appendChild(h("b", {}, fmt(last, o.decimals, o.unit)));
    }

    // y domain
    let lo = Infinity, hi = -Infinity;
    for (const s of data) for (const se of series) {
      const v = se.get(s);
      if (!isNum(v)) continue;
      if (v < lo) lo = v; if (v > hi) hi = v;
    }
    if (!isFinite(lo)) { lo = o.min != null ? o.min : 0; hi = o.max != null ? o.max : 1; }
    if (o.zero) { lo = Math.min(lo, 0); hi = Math.max(hi, 0); }
    if (o.min != null) lo = Math.min(lo, o.min);
    if (o.max != null) hi = Math.max(hi, o.max);
    const minSpan = o.minSpan || 1;
    if (hi - lo < minSpan) {
      if (o.zero && lo >= 0) hi = lo + minSpan;          // all positive: grow upwards from 0
      else if (o.zero && hi <= 0) lo = hi - minSpan;     // all negative: grow downwards from 0
      else { const c = (hi + lo) / 2; lo = c - minSpan / 2; hi = c + minSpan / 2; }
    }
    const step = niceStep(hi - lo, Math.max(2, Math.round(ph / 40)));
    lo = Math.floor(lo / step) * step; hi = Math.ceil(hi / step) * step;
    const x = (t) => m.l + (t - t0) / (t1 - t0) * pw;
    const y = (v) => m.t + ph - (v - lo) / (hi - lo) * ph;
    this.x = x; this.y = y;
    if (!this.clipId) this.clipId = "clip" + Math.random().toString(36).slice(2);
    const clip = el("clipPath", { id: this.clipId }, el("defs", {}, svg));
    el("rect", { x: m.l - 6, y: m.t - 6, width: pw + 12, height: ph + 12 }, clip);

    // grid + y ticks
    const decimalsTick = step < 0.1 ? 2 : Number.isInteger(step) ? 0 : 1;
    for (let i = 0, v = lo; v <= hi + step / 2; i++, v = lo + i * step) {
      if (Math.abs(v) < step / 1e6) v = 0;   // no "-0.0" from rounding
      const yy = Math.round(y(v)) + 0.5;
      el("line", { x1: m.l, x2: m.l + pw, y1: yy, y2: yy, stroke: Math.abs(v) < step / 2 && o.zero ? css("--axis") : css("--grid"), "stroke-width": 1 }, svg);
      const big = Math.abs(hi) >= 10000 || Math.abs(lo) >= 10000;
      const lbl = big ? (v / 1000).toFixed(step < 1000 ? 1 : 0) + "K"
                      : v.toLocaleString(undefined, { minimumFractionDigits: decimalsTick, maximumFractionDigits: decimalsTick });
      el("text", { x: m.l - 8, y: yy + 4, "text-anchor": "end", class: "tick" }, svg).textContent = lbl;
    }

    // x ticks: at most one per ~70 px, on whole local minutes/hours/days
    const spanMin = (t1 - t0) / 60000;
    const want = Math.max(2, Math.floor(pw / 70));
    const xStepMin = X_STEPS_MIN.find((s) => spanMin / s <= want) || 1440 * Math.ceil(spanMin / 1440 / want);
    const tzOff = new Date(t0).getTimezoneOffset() * 60000;
    const xStep = xStepMin * 60000;
    for (let t = Math.ceil((t0 - tzOff) / xStep) * xStep + tzOff; t <= t1; t += xStep) {
      const xx = x(t);
      if (xx < m.l + 14 || xx > m.l + pw - 14) continue;
      const d = new Date(t);
      const midnight = d.getHours() === 0 && d.getMinutes() === 0;
      el("text", { x: xx, y: H - 4, "text-anchor": "middle", class: "tick" }, svg).textContent =
        (xStepMin >= 1440 || (midnight && spanMin > 1440)) ? dayLabel(t) : clock(t);
    }
    el("line", { x1: m.l, x2: m.l + pw, y1: m.t + ph + 0.5, y2: m.t + ph + 0.5, stroke: css("--axis"), "stroke-width": 1 }, svg);

    if (!data.length) {
      el("text", { x: m.l + pw / 2, y: m.t + ph / 2, "text-anchor": "middle", class: "empty" }, svg).textContent =
        this.emptyText || "No data for this time range";
      return;
    }

    // lines
    const gapMs = o.gapMs || Infinity;
    for (const se of series) {
      let d = "", prevT = null, prevY = null;
      for (const s of data) {
        const v = se.get(s);
        if (!isNum(v)) { prevT = null; continue; }
        const xx = x(s.t).toFixed(1), yy = y(v).toFixed(1);
        if (prevT == null || s.t - prevT > gapMs) d += "M" + xx + "," + yy;
        else if (o.step) d += "H" + xx + "V" + yy;
        else d += "L" + xx + "," + yy;
        prevT = s.t; prevY = yy;
      }
      // A step line holds its last value up to the end of a live window.
      if (o.step && prevT != null && o.holdTo) d += "H" + x(Math.min(o.holdTo, t1)).toFixed(1);
      if (d) el("path", { d, "clip-path": "url(#" + this.clipId + ")", fill: "none", stroke: css(se.color), "stroke-width": 2,
                          "stroke-linejoin": "round", "stroke-linecap": "round", "stroke-dasharray": DASHES[se.dash || "solid"] }, svg);
    }
    // end dots
    const last = data[data.length - 1];
    for (const se of series) {
      const v = se.get(last);
      if (!isNum(v)) continue;
      el("circle", { cx: x(last.t), cy: y(v), r: 4, fill: css(se.color), stroke: css("--surface-1"), "stroke-width": 2 }, svg);
    }
    this.hoverLayer = el("g", {}, svg);
    if (this.hoverIdx != null) this.showIdx(Math.min(this.hoverIdx, data.length - 1));
  };

  LineChart.prototype.nearest = function (t) {
    const d = this.data; if (!d || !d.length) return null;
    let lo = 0, hi = d.length - 1;
    while (hi - lo > 1) { const mid = (lo + hi) >> 1; if (d[mid].t < t) lo = mid; else hi = mid; }
    if (this.o.step) return d[hi].t <= t ? hi : lo; // the value in force at t
    return Math.abs(d[lo].t - t) <= Math.abs(d[hi].t - t) ? lo : hi;
  };
  LineChart.prototype.onMove = function (e) {
    if (!this.geom || !this.data || !this.data.length) return;
    const r = this.svg.getBoundingClientRect(), g = this.geom;
    const px = (e.clientX - r.left) * (g.W / r.width);
    if (px < g.m.l || px > g.m.l + g.pw) { this.hide(); return; }
    const t = g.t0 + (px - g.m.l) / g.pw * (g.t1 - g.t0);
    const idx = this.nearest(t);
    if (idx == null) { this.hide(); return; }
    this.showIdx(idx);
  };
  LineChart.prototype.onKey = function (e) {
    if (!this.data || !this.data.length) return;
    let i = this.hoverIdx == null ? this.data.length - 1 : this.hoverIdx;
    if (e.key === "ArrowLeft") i = Math.max(0, i - 1);
    else if (e.key === "ArrowRight") i = Math.min(this.data.length - 1, i + 1);
    else if (e.key === "Escape") { this.hide(); return; }
    else return;
    e.preventDefault();
    this.showIdx(i);
  };
  LineChart.prototype.showIdx = function (idx) {
    this.hoverIdx = idx;
    const s = this.data[idx], g = this.geom, layer = this.hoverLayer;
    if (!s || !layer) return;
    while (layer.firstChild) layer.removeChild(layer.firstChild);
    const xx = this.x(Math.max(s.t, g.t0));
    el("line", { x1: xx, x2: xx, y1: g.m.t, y2: g.m.t + g.ph, stroke: css("--axis"), "stroke-width": 1 }, layer);
    const tip = this.tip;
    tip.textContent = "";
    const long = g.t1 - g.t0 > 86400000;
    tip.appendChild(h("div", { class: "t" }, (long ? dayLabel(s.t) + " " : "") + clockS(s.t)));
    for (const se of this.series) {
      const v = se.get(s);
      if (isNum(v)) el("circle", { cx: xx, cy: this.y(v), r: 4, fill: css(se.color), stroke: css("--surface-1"), "stroke-width": 2 }, layer);
      const row = h("div", { class: "row" });
      row.appendChild(swatch(se.color, se.dash));
      row.appendChild(h("b", {}, fmt(v, this.o.decimals, this.o.unit)));
      row.appendChild(h("span", {}, se.label));
      tip.appendChild(row);
    }
    tip.style.display = "block";
    const r = this.svg.getBoundingClientRect(), scale = r.width / g.W;
    const tw = tip.offsetWidth;
    let left = xx * scale + 12;
    if (left + tw > r.width) left = xx * scale - tw - 12;
    tip.style.left = Math.max(0, left) + "px";
    tip.style.top = (r.top - this.c.getBoundingClientRect().top + g.m.t * scale) + "px";
  };
  LineChart.prototype.hide = function () {
    this.hoverIdx = null;
    this.tip.style.display = "none";
    if (this.hoverLayer) while (this.hoverLayer.firstChild) this.hoverLayer.removeChild(this.hoverLayer.firstChild);
  };

  // ---------- live data ----------
  // stream connects to a Server-Sent Events URL and calls onMessage with each
  // parsed JSON event. onState(kind, text) reports the connection: "ok"
  // while messages arrive, "warn" when none has arrived for staleMs, "error"
  // when the connection is lost. onReconnect is called when the connection
  // comes back after being lost, so a page can reload anything it missed.
  // Browsers without EventSource poll pollUrl instead.
  function stream(opts) {
    const staleMs = opts.staleMs || 30000;
    let lastMsg = 0, lost = false, state = "";
    const report = (kind, text) => { const k = kind + text; if (k !== state) { state = k; if (opts.onState) opts.onState(kind, text); } };
    const handle = (data) => {
      lastMsg = Date.now();
      report("ok", "Live");
      opts.onMessage(data);
    };
    setInterval(() => {
      if (lastMsg && Date.now() - lastMsg > staleMs) report("warn", "No update for " + Math.round((Date.now() - lastMsg) / 1000) + " s");
    }, 5000);

    if (!window.EventSource) {
      const poll = () => {
        fetch(opts.pollUrl, { cache: "no-store" })
          .then((r) => r.ok ? r.json() : Promise.reject(r.status))
          .then(handle)
          .catch(() => report("error", "Connection to the controller lost"))
          .finally(() => setTimeout(poll, opts.pollMs || 5000));
      };
      poll();
      return;
    }
    report("warn", "Connecting…");
    const es = new EventSource(opts.url);
    es.onopen = () => {
      if (lost && opts.onReconnect) opts.onReconnect();
      lost = false;
    };
    es.onmessage = (e) => {
      let data;
      try { data = JSON.parse(e.data); } catch (err) { console.log("Bad event: " + err); return; }
      handle(data);
    };
    es.onerror = () => {
      lost = true;
      report("error", "Connection to the controller lost; retrying");
    };
  }

  window.SC = { $, css, el, h, fmt, fmtW, isNum, clock, clockS, dayLabel, swatch, icon, setStatus, LineChart, stream };
})();
