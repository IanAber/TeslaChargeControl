# EG4

Go reader for paralleled EG4 12000XP inverters over Modbus TCP (Waveshare in "Modbus TCP to RTU" mode). No external dependencies.

```go
sys, err := EG4.NewSystem(
    EG4.Config{Name: "XP-1", Address: "192.168.10.201:502", UnitID: 1},
    EG4.Config{Name: "XP-2", Address: "192.168.10.202:502", UnitID: 1},
)
defer sys.Close()
sr, err := sys.Read(ctx) // sr.Units[i] per inverter, sr.PVPower etc. totals
```

- Each inverter on its own Waveshare channel: give each its own Address; they are read concurrently.
- Both on one RS485 bus: same Address, different UnitID (set holding register 15 on one inverter). Requests are serialized.
- Each poll does five FC04 reads (input 0-199). Keep `Timeout` above the Waveshare instruction timeout.
- Single inverter: `EG4.NewInverter(cfg).Read(ctx)`. Raw access: `ReadInputs` / `ReadHolding`.

SystemController configures the inverters with `-inverters`, a comma separated list of IP addresses (Modbus TCP port 502, unit ID 1). The first address is XP-1, the second XP-2, and so on:

```
SystemController -inverters 192.168.10.201,192.168.10.202
```
