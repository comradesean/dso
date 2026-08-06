# `0x038B RegulationFileUpdatePushMessage` — live param replacement

**Status: CONFIRMED WORKING END TO END on a live client (2026-08-06).** We pushed a replacement
`regulationEnglish.fmg`, the client accepted it, and the applier wrote our `version_new` into the
holder — 11500 became 11501 in memory. The whole chain from push dispatcher to applier is now
observed behaviour, not inference.

All addresses are v1.10 vaddrs unless noted. Everything marked CONFIRMED was read out of the
disassembly; INFERRED and UNKNOWN are called out.

---

## Summary for the impatient

- `diff_data` is **not a diff**. It is the raw bytes of **one whole resource file** — no BND4, no
  DCX, no zlib, no delta format.
- The param route requires the payload size to **equal the loaded resource's size exactly**.
- `start_at` / `end_at`: **no reader found**, but treat that as unresolved rather than dead — the
  handler defaults them to meaningful "always valid" sentinels, which argues something reads them.
- `importance` (field 1) is the **new** regulation version; `target_regulation_version` (field 2) is
  the **prerequisite**. It is a version chain, not a priority.
- Applied **every frame** from the per-tick update, in place, in the live resource.

## The message

```proto
RegulationFileUpdatePushMessage:
  1 push_message_id
  2 update_msg -> RegulationFileUpdateMessage { 1 diff_data_list (repeated RegulationFileDiffData) }

RegulationFileDiffData:
  1 importance             2 target_regulation_version
  3 path (string)          4 diff_data (bytes, INLINE)
  5 start_at (DateTime)    6 end_at (DateTime)      7 (varint, never read)
```

Handler `0x15F74B8`.

## The listener at manager+76 — FOUND (CONFIRMED)

Two previous investigations failed here. The reason it was invisible: the store lives **inside the
netlib**, the caller passes a **heap-allocated bound delegate**, and the member-function pointer is
only reachable through a **GOT slot**. No search keyed on the displacement or on a direct call could
have found it.

The handler's `this` is the netlib *PushMessageManager* at `session+8`. Its ctor `0x15F3DDC` nulls
three listener slots:

```
15f3e70: stw r9,80(r27)   ; +80 = 0x038C listener
15f3e74: stw r9,72(r27)   ; +72 = 0x0389 listener (banner)
15f3e78: stw r9,76(r27)   ; +76 = 0x038B listener (regulation)
```

Cross-checks: `0x15F4448 lwz r3,72(r30)` in the `0x0389` handler, `0x15F4738 lwz r3,80(r30)` in the
`0x038C` handler, and the manager dtor `0x15F5218` releases all three via `vtbl[+4]`.

**The setter is `0x15F49F8`:**

```
15f4a8c: lwzu r9,76(r11)   ; r11 = this+76, r9 = old listener
15f4a94: stw  r0,0(r11)    ; clear
15f4aa0: stw  r8,0(r11)    ; *(this+76) = new listener
```

Reached as netlib-session vtable slot **+88** (`vtable 0x1CE0B20[+88] = 0x15C20A0`, which does
`addi r3,r31,8; bl 0x15F49F8`).

**Game-side installer `0x6DDF14`** builds a 16-byte bound delegate — the same idiom the netlib uses
at `0x15F5174`:

```
6ddf14: lwz r0,-32736(r30)   ; = 0x1D55258 = OPD of 0x6DC598  (member fn ptr)
6ddf1c: lwz r10,-32744(r30)  ; delegate vtable
6ddf24: stw r0,8(r9)         ; delegate.memfn
6ddf2c: stw r31,4(r9)        ; delegate.this = 68-byte session wrapper
6ddf30: stw r10,0(r9)
6ddf50: bctrl                ; session->vtbl[88](session, &delegate)
```

**Delegate target `0x6DC598`:**

```
6dc5a8: lwz r0,56(r3)        ; wrapper->m_regulationListener
6dc5b8: beq  ...             ; null -> nothing
6dc5c0: lwz r11,4(r9)        ; listener->vtbl[+4]
6dc5d4: bctrl                ; listener->OnUpdate(vec)
```

`wrapper+56` is set at `0x6E2F58` (`addi r4,r26,8; lwz r3,52(r28); bl 0x6DC50C`, where
`0x6DC50C = stw r4,56(r3)`). `r26` is a 128-byte game network manager (ctor `0x6E28E0`, vptr trio
`0x1CA7680 / +0x100 / +0x120`); `+8` is its third base, vtable `0x1CA77A0[+4] = 0x6E0688` → thunk(-8)
→ **`0x6E0690`**, which discards `this`, keeps the vector, and calls `0x772420`.

## The holder

**`0x772420` = `RegulationDiffHolder::Append(vector<DiffData>&)`.** Takes the holder mutex
(`singleton+8`, `vtbl[+12]`), then appends each 88-byte record into the singleton's vector at `+40`
(`mulli r0,r20,88` at `0x772538`, `addi r11,r26,88` at `0x772658`). Requires ≥1 element
(`0x7724DC: addi r9,r9,87; cmplwi 174; ble → exit`).

**Singleton pointer global `0x1E1D388`.** Created `0x772344` (56 bytes, ctor `0x76F43C`), destroyed
`0x7722A0`, list cleared `0x771EE0` / `0x7720D0`.

Layout: `+0` current regulation version, `+8` mutex, `+40` `vector<DiffData>`.

**88-byte record layout** (CONFIRMED from `0x15F7750`–`0x15F7AF0`):

| off | size | field |
|---|---|---|
| +0 | 4 | field 1 (`importance`) |
| +4 | 4 | field 2 (`target_regulation_version`) |
| +8 | 32 | `path` (wide-string; +4 inline buf, +20 size, +24 cap, +28 flag) |
| +40 | 16 | `diff_data` byte vector {alloc, begin, end, cap} |
| +56 | 16 | `start_at` |
| +72 | 16 | `end_at` |

## What `diff_data` must contain (CONFIRMED)

Applier **`0x76FE84`**. Two terminal routes.

**`.fmg` route — `0x76A0F0`:** (the size annotation below was originally written backwards; `ble`
takes the **work** branch, so <= 1024 is ACCEPTED and > 1024 returns 0)

```
76a108: cmpwi r0,1024        ; size
76a11c: ble   0x76a13c       ; <= 1024 -> do the work; otherwise fall through, return 0
76a140: lwz   r3,8(r29)      ; r29 = res+144  => dst = *(res+152)
76a144: bl    0x184DD1C      ; memcpy(dst, data, size)
76a158..68: *(dst+20) += dst ; relocate one embedded offset->pointer
76a174: stb   r0,164(r31)    ; resource dirty flag = 1
```

**param route — size must match exactly, then full reload:**

```
770d9c: lwz r0,8(r28)        ; r28 = res+140
770dbc: lwz r31,12(r28)      ; existing resource size
770dd0..de0: r0 = diff_data.end - diff_data.begin
770de4: cmpw r0,r31
770de8: beq  0x771200        ; sizes must be EQUAL, else the entry is skipped
...
771220: bl 0x76EF30          ; data()
771270: bl 0xBB2B60          ; memory-source ctor
771288: bl 0xBB2CCC          ; source.set(data, size, 0)
77129c: bl 0xC22848(res+140, source, 1)  -> 0xBB3060 = load resource from memory
```

**There is no BND4/DCX/PARAM magic check, no CRC, no zlib/inflate, and no delta decoder anywhere on
this path.** The name "diff" is a misnomer at the client end — it is a whole-resource replacement.

## What `path` selects — the two branches are ASYMMETRIC (CONFIRMED)

**This is the trap.** For a param the client prepends `param:/` to whatever you send, so a bare
`OnlineEventParam.param` is correct. **For an FMG it prepends nothing** — the lookup uses your string
as-is, so you must send the resource's *full* path.

And an FMG's resource path is nothing like its name inside the archive. The BND4 entry is
`regulationEnglish.fmg`, but the loaded resource is registered from the template at `0x1881580`:

```
text:/Text/%s/regulation.fmg        %s = "English" (literal at 0x1888F08)
```

so the key is **`text:/Text/English/regulation.fmg`**. Sending `regulationEnglish.fmg` can never
match, which is why our first obelisk push did nothing. Siblings at `0x18813E0`
(`text:/Text/%s/Staffroll.fmg`) and `0x18815C0` (`text:/Text/%s/%s.fmg`) confirm the shape.

### The branch, disassembled

```
770800: bl 0xde50f8    ; GetExtension(path)
770810: bl 0x184df2c   ; wcscmp against L".fmg" @ 0x1871570
77081c: bne 0x770c08   ; not .fmg -> param route (prepends param:/)
770830: bl 0x76b514    ; resource type = MessageResourceObject_ForRegulation
                       ;   (class string @ 0x1871400, truncated to "ageResourceObject_ForRegulation")
770844: stw r31,128(r1); lookup key = {path, type}
770848: bl 0xc169e8    ; case-folding repository lookup
770850: cmpwi r3,0
770854: mr r31,r3
770858: beq 0x7708c4   ; NOT FOUND -> SKIP, silently, and carry on with the loop
7708bc: bl 0x76a0f0    ; apply: memcpy into the live buffer
```

That `beq` at `0x770858` is the whole failure: a missed lookup costs nothing and reports nothing.

### It also means the version bump proves less than it looks

`current_version` is written at `0x770480`, **after** the entry loop, and records that an entry
passed the version and bounds checks — not that its resource was found or overwritten. So an
incremented counter with no visible effect is exactly what a wrong `path` produces. The chest is the
only case where we have independent proof of application.

## What `path` selects — original notes (superseded above for FMGs)

`path` is copied to a wide string, separator-normalised (`0x43D60C`, `0xE721A8` find, `0xE72070`
replace), then:

- `GetExtension(path)` (`0xDE50F8`) is compared with `L".fmg"` (UTF-16BE literal at `0x1871570`) via
  `0x184DF2C`→`0x2813EC`.
- If not `.fmg`, the applier **prepends `L"param:/"`** (literal at `0x1871580`, concatenated at
  `0x770C3C`–`0x770D5C`) and looks the result up in the resource repository via `0xC169E8`
  (case-folding wide-string lookup; repo = `*(*(0x1E1D810)) + 24`).

**CONFIRMED for params:** send `path = "OnlineEventParam.param"` bare — the client prepends
`param:/` itself, and this is the form that armed the chest. **For FMGs send the full resource
path**, per the section above.

## `start_at` / `end_at` — no comparison found in this module (TREAT AS UNRESOLVED)

Across the whole holder/applier module (`0x76E000`–`0x773500`), every access to element `+56` and
`+72` is a 16-byte copy in a copy-ctor, assign, or vector-grow (`0x76F640`, `0x76FA64`, `0x7718FC`,
`0x771CA4`, `0x772624`, `0x7727C8`, `0x772BD0`, `0x772D2C`, `0x7730B0`). **No compare, no `cellRtc`
call, no arming or disarming.** The applier `0x76FE84` contains zero references to those offsets.

**Do not treat this as "the fields are dead."** Two reasons to keep it open:

- **The handler goes out of its way to default them** to 2000-01-01 and 2100-01-01 via `cellRtc`.
  Those are "always valid" sentinels. Zeroing would satisfy an uninitialised-parse concern for free;
  constructing meaningful dates costs work that only pays off if something compares them. That is
  evidence *for* a reader, from inside the same investigation that failed to find one.
- **The search shape cannot see the likely pattern.** These are 16-byte structs, so a comparison
  takes their address — `addi r3,r26,56; bl <compare>` — and the comparator may live anywhere. To an
  offset scan, address-taken-and-passed is indistinguishable from a struct copy. The scan was also
  scoped to one module (`0x76E000`–`0x773500`); the poll `0x771330`, `Append`, and the applier gate
  `obj->vtbl[+200]` are all plausible homes for a window check.

This is the same class of miss that declared the bell receive path dead: a store encoded as
`stw rS,0(rA)` after an `lwzu` advanced the base, invisible to every search keyed on the
displacement. See `tasks/bell-broadcast.md`.

So: **no comparison was found**, which is not the same as none existing. The earlier theory in
`tasks/majula-event-chest.md` that per-diff activation windows scheduled the weekly rotation is
unsupported, but it is not refuted either.

## `importance` and `target_regulation_version` — a version chain (CONFIRMED)

```
7705A8: lwz r9,0(r18)        ; holder->current_version
7705AC: lwz r0,4(r3)         ; elem.target_regulation_version
7705B0: cmpw
7705B4: bne -> next entry    ; must match (check skipped if current_version == 0)

77040C: lis r9,15 ; ori r9,r9,16959   ; 999999
770410: lwz r0,0(r3)         ; elem."importance"
770418: cmplw ; bgt -> skip  ; must be <= 999999
770420: beq cr4 -> accept    ; cr4 = (r14 == -1), i.e. nothing accepted yet
770438: cmplw r0,r14 ; ble -> skip     ; must be strictly greater
770450: lwz r14,0(r3)        ; r14 = new best

770480: stw r14,0(r9)        ; holder->current_version = "importance"
```

`holder->current_version` is written from field 1 and compared against field 2, so **field 1 is the
resulting version and field 2 is the prerequisite**. Corroborating: `0x76A384` (regulation-version
getter) uses the same `999999` bound. `current_version` is seeded at regulation load
(`0x76C8D0` → `0x76E7E8`, `stw r11,0(singleton)`) and is 0 from the ctor (`0x76F468`), in which case
the field-2 check is skipped.

A mismatch **rejects the entry silently** (jumps to the loop increment at `0x770458`).

## It reaches live params, no restart (CONFIRMED for the mechanism)

- `0x771330` is the poll: checks the holder exists, list has ≥1 entry, plus a gate `obj->vtbl[+200]`,
  then calls the applier `0x76FE84`.
- `0x771330` ← `0x760C34` ← `0x56E1A0`, inside a per-tick update (frame counter
  `lwz r9,148(r31); addi r9,r9,1; stw` at `0x56E17C`, dt in `f31`).

A push arriving mid-session is applied on the next tick. The param route **reloads the live resource
object in place** (`0xC22848` → `0xBB3060`); the FMG route memcpys straight into the loaded buffer and
sets a dirty flag. After a successful pass the whole diff list is destroyed
(`0x77049C`–`0x7704B8`), so each push applies once.

## What to send

```
RegulationFileUpdatePushMessage {
  push_message_id: <as usual>
  update_msg { diff_data_list: [ {
      importance:                <client_regulation_version + 1>   // becomes the new version, <= 999999
      target_regulation_version: <client_regulation_version>       // must equal what the client loaded
      path:                      "OnlineEventParam.param"          // client prepends "param:/"
      diff_data:                 <entire new PARAM file, byte for byte>
      // start_at / end_at / field 7: omit, never read
  } ] }
}
```

The client reports `regulation_version` in `PlayerStatus` via `RequestUpdatePlayerStatus` (`0x03B8`),
which is where both version numbers come from.

Hard constraints from the disassembly:

- **Payload size must equal the currently loaded resource's size exactly** (`0x770DE4`). You cannot
  add or remove rows. Flipping the `u16` at `OnlineEventParam` row 0 `+2` is same-size, and so is
  rewriting an existing `ItemLotParam2_SvrEvent` row — but you **cannot create** a row that is not
  already present.
- Content is a whole raw resource. No container, no compression.
- Only **one entry** effectively applies per pass unless entries are ordered by strictly increasing
  field 1 with field 2 chained correctly.
- A `.fmg` payload is capped at **1024 bytes**.

## IMPLEMENTED — how to run the live test

`internal/server/game/regulationpush.go`, sent from the login handler right after the management
text. Off unless `DSO_REGULATION_PUSH_FILE` is set.

The payload is already built: **`data/regpush/OnlineEventParam.armed.param`** is 0114's own
`OnlineEventParam.param` with the claim threshold at row 0 `+2` raised from `0` to `1`, and nothing
else touched. `data/regpush/OnlineEventParam.param` is the untouched original for comparison, and
`TestRegulationPushPayloadSizeUnchanged` enforces that the two differ in exactly those two bytes and
not in length.

```
DSO_REGULATION_PUSH_FILE=data/regpush/armed/OnlineEventParam.param
DSO_REGULATION_PUSH_VERSION_REQUIRED=11500   # the client's build version, read from the holder
DSO_REGULATION_PUSH_DELAY_SECONDS=0
```

Build a different payload with `tools/gamedata/regparam.py` — it refuses to change a file's length,
which is the constraint that otherwise fails in silence.

**Procedure.** The chest's arm method `0x58E360` is not known to re-run on its own; it may only fire
when the object is registered at map load. So: log in somewhere other than Majula, confirm the push
in the server log, then travel to Majula so the map loads *after* the data changed, and open the far
chest (object `10045510`, the pair-mate of the ordinary Soul Vessel one).

**Reading the result.**

- **Chest gives something** → the applier's repository and the chest's are the same, `0x038B` works
  end to end, and open item 1 below is answered.
- **Nothing** → try `DSO_REGULATION_PUSH_PATH=param:/OnlineEventParam.param` (open item 2), then
  suspect open item 1 or GATE A in `tasks/majula-event-chest.md`.

Everything about this push fails **silently** on the client — wrong size, wrong path, wrong version,
wrong repository all look identical from outside. A negative result therefore isolates nothing on its
own; change one variable at a time.

## CONFIRMED WORKING — the accept, observed

Second read, after a push carrying `version_required = 11500`:

```
+0x00  0x00002CED = 11501      <- was 11500; the applier wrote our version_new
```

**That is an accepted, applied entry.** Confirmed by it: `version_required` must equal the game's
build version; a bare `regulationEnglish.fmg` is accepted as `path` with no `param:/` prefix needed
for FMGs; the applier runs and writes field 1 back into the holder exactly as `0x770480` says.

The obelisk text did **not** visibly change, and that is now explained rather than mysterious: the
FMG route memcpys into `*(res+152)`, the buffer the file was loaded into, and Majula had already
copied the string into its own display state. **A visible-text probe is therefore not a valid test
of this message** — it can read "no change" on complete success. `current_version` is the correct
readout, because the applier writes it before any display is involved.

### `current_version` moves only if we let it

It is 11500 at boot (seeded from the loaded regulation) and becomes whatever `version_new` we sent
after each accepted push.

**So we send `version_new = version_required` and it never moves.** Incrementing looks natural —
FromSoftware presumably chained diffs that way — and for us it is a trap: we send up to three pushes
per login, so the counter would climb ~3 each time and walk off a sweep window after three or four
logins, at which point everything stops with no error anywhere. Widening the window does not fix it,
because every entry carries a full copy of the payload and the lot param alone is 4420 bytes.

Nothing forbids holding it steady. The only bounds on field 1 are `<= 999999` (`0x770418`) and
strictly-greater-than-the-best-accepted-**within one pass** (`0x770438`, accumulator starts at `-1`),
and at most one entry per push can match a single stored value, so that second rule never binds.

The sweep still exists to cover the value itself: it is 11500 after a game restart, but the holder
may or may not survive a network logout (`0x7722A0` destroys it on teardown), so the window absorbs
both cases.

## SOLVED BY DEBUGGER — `current_version` is the client's build version

**Read live from RPCS3 (2026-08-06).** `0x1E1D388` held `0x312813B0`; dumping the holder there:

```
+0x00  0x00002CEC = 11500      <- current_version  (game build 1.15)
+0x04  0x00002C24 = 11300      <- second version-shaped field (1.13), unidentified
+0x08  0x01CBADB8              <- vtable pointer (the mutex object)
+0x28  allocator 0x30A00F1C
+0x2C  begin     0x312878A0
+0x30  end       0x312878A0    <- begin == end, EMPTY
+0x34  cap       0x31288710
```

Two things settled at once.

**The version to send is 11500**, the game build number — 1.15, exactly as shown in the game's own
menu. Not a calibration number, not the manifest's `Version` field (which is 1 in every published
manifest and carries no information). Nothing in the protocol reports it, which is why three live
tests sending `0` were rejected at the first comparison and looked identical to every other failure.

**The whole receive chain is CONFIRMED WORKING.** Capacity is `0x31288710 - 0x312878A0` = 3696 bytes
= exactly **42 x 88**, the record stride. A 1.5x-growth vector reaches 42 via `...19, 28, 42` — which
is what appending the 30 sweep entries produces. So the vector grew to hold our entries and was then
emptied: the push arrives, parses, the listener at `manager+76` is installed and firing, `Append`
runs, and the applier runs and clears the list. Every entry was rejected at the version check and
nothing else.

This also **confirms the agent's identification of the holder global**, which had been inference from
the referencing code: the layout at `0x1E1D388`'s target matches the recovered one field for field.

Method note: three live tests produced "no change" and distinguished nothing between them, because
every failure mode in this message is silent and identical from outside. One memory read produced
both the answer and a proof that everything upstream works. When the unknown is the contents of a
global rather than a branch, reach for the debugger first — the same lesson the bell taught.

## STILL OPEN — the obelisk

The chest works; the obelisk does not. With `path = text:/Text/English/regulation.fmg` the text is
still the stock "The letters are worn beyond recognition."

What is ruled out:

- **Size.** `0x76A0F0` accepts <= 1024 and our payload is 128. Had it been found, the memcpy and the
  `+20` relocation would have rewritten exactly the bytes we want.
- **The language token.** The table at `0x1888EF0`-`0x1888F98` is `Japanese, English, French,
  Germany, Italian, Spanish, Korean, Chinese, Russian, Polish, Portuguese` — exactly the eleven
  `regulation<Lang>.fmg` files in the archive. `English` is right.
- **The version.** The same sweep lands the chest's params in the same session.

So the repository lookup at `0xC169E8` is returning NULL and `0x770858` skips the apply. Either the
resource is registered under a different key, or the regulation FMG is not registered as a resource
at all and the game copies its strings elsewhere at load.

Sibling templates use other schemes — `dlc_data:/Menu/Text/%s/bloodmes/%s.fmg` (`0x1888E30`),
`gamedata_patch:/Menu/Text/%s/bloodmes/%s.fmg` (`0x1888E80`), `title_patch:/param/` (`0x1871460`) —
so a patched 1.10 install may register it under one of those. `DSO_REGULATION_PUSH_PATH` now takes a
comma-separated list and sends one push per candidate.

**The decisive test is a memory search**, not more guessing. In RPCS3's memory viewer, search the
UTF-16BE bytes of `regulation.fmg`:

```
0072006500670075006c006100740069006f006e002e0066006d0067
```

and read the whole wide string around each hit — that is the key as actually registered. Searching
for the obelisk text itself confirms which side of the lookup failed:

```
0054006800650020006c006500740074006500720073002000610072006500200077006f0072006e
```

Present and unmodified means the buffer is loaded and our memcpy never ran.

## OPEN — resolve before building a payload

1. **Repository identity — the big one.** The applier reaches its repo via `*(*(0x1E1D810)) + 24`.
   The chest's threshold reader `0x66F1D8` reads `*(*(0x1E1EAB4 + 32)) + 24`. **Different globals.**
   If they are not the same repository, a push lands somewhere the chest never looks and the whole
   plan fails silently. UNKNOWN.
2. **`path` spelling** — bare vs `param:/`-prefixed. The normalisation stage `0x76FF50`–`0x770200`
   is undecoded. Cheap to test both.
3. **Route A.** Before the extension test, if `r16 = *(repo+172) != 0` the entry goes to
   `0x76BB30(r16, name, data)` instead (`0x76FF48` / `0x7705B8`). Undecoded — UNKNOWN whether it also
   applies or merely registers an overlay, and UNKNOWN whether that pointer is non-null at runtime.
4. **v1.00 not cross-checked** beyond confirming the `L"param:/"` and `L".fmg"` literals exist
   (`0x17FD600` / `0x17FD4C8`).

## Why this reframes the chest

`OnlineEventParam` row 0 and `ItemLotParam2_SvrEvent[10045500]` are byte-identical across all ten
published calibrations, including two dated on event days — yet the event demonstrably ran. This
message explains that cleanly: **FromSoftware pushed whole param files at runtime over `0x038B`**,
so the weekly data never had to appear in the S3 calibration channel at all. Nothing needs to be
scheduled client-side, which is consistent with `start_at`/`end_at` being dead.

See `tasks/majula-event-chest.md` for the chest gate itself (`OnlineEventParam` row 0, `u16` at `+2`).
