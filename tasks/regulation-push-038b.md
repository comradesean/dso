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
- `path` is the key the resource is **registered** under, which is a bare filename for both routes:
  `OnlineEventParam.param`, `regulation.fmg`. Not the BND entry name, not the load path.

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
as-is, so it must equal the key the resource was registered under.

> **That key is `regulation.fmg`, bare — read out of live memory 2026-08-07. See "SOLVED — the
> obelisk key" below.** The paragraph that used to stand here inferred
> `text:/Text/English/regulation.fmg` from the format template at `0x1881580`, and that inference was
> wrong: the template is the *load* path, not the *registry* key. The template string is resident in
> the string pool right next to the key, which is exactly how it fooled us.

Siblings at `0x18813E0` (`text:/Text/%s/Staffroll.fmg`) and `0x18815C0` (`text:/Text/%s/%s.fmg`)
confirm the template shape — but not that any of them is a repository key.

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
      path:                      "OnlineEventParam.param"          // the REGISTERED key, bare;
                                                                   // "regulation.fmg" for the obelisk
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
- **Exactly one entry applies per pass, and the rest are destroyed** (CONFIRMED). `0x770454`
  recomputes `cr4` after each accept, so the strictly-greater test at `0x770438` is live for every
  entry after the first; then `0x77049C` destroys the whole list, rejected entries included. Since we
  deliberately hold field 1 steady, a second entry at the same version is always dropped. **This
  applies across pushes, not just within one** — two pushes arriving in the same frame are appended
  to the same vector and only one survives. Space them apart in time
  (`DSO_REGULATION_PUSH_GAP_SECONDS`).
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

## HAZARD — a wrong `.fmg` path can CRASH the client

**Learned the hard way, 2026-08-06: the game crashed twice on load after we sent nine candidate FMG
paths in one login.**

The two routes fail very differently:

| | wrong path that misses | wrong path that HITS |
|---|---|---|
| param | lookup fails, entry skipped, harmless | size must match exactly or it is skipped — also harmless |
| **fmg** | lookup fails, harmless | **memcpy of your bytes over that resource, NO size check** |

`0x76A0F0` checks only that the payload is <= 1024 bytes. It does **not** compare against the
destination's size. It memcpys into `*(res+152)` and relocates `*(dst+20)`. So overwriting a large
text resource with a small FMG leaves everything past your payload stale and the string offset table
pointing into garbage — and the client dies when it next reads that text, which is during a load.

**Do not shotgun candidate paths for FMGs.** One at a time, chosen for a reason. Params are safe to
sweep; FMGs are not.

Recovery is a restart: the apply path writes only to memory, so nothing survives the process.

## SOLVED — the obelisk key is `regulation.fmg`, bare (2026-08-07)

**The path we had been sending was wrong.** The regulation FMG is registered in the resource
repository under the bare filename **`regulation.fmg`** — not `regulationEnglish.fmg` (its BND4 entry
name) and not `text:/Text/English/regulation.fmg` (the template-derived load path we inferred and
sent). Read straight out of a live RPCS3, so this is observation, not inference.

### How it was recovered — pointer-walking, not searching

The previous pass stopped at a 6-word memory block and guessed the key was one indirection further
out. It was not: that block is a heap allocation descriptor, and the key hangs off a *different*
object. Two facts from the disassembly turned the search into a walk:

- **`0x76A0F0` says the resource object holds its buffer at `+152`** (`addi r29,r31,144;
  lwz r3,8(r29)`) and its dirty flag at `+164`.
- **`0xC169E8` says the same object holds its name at `+4`** — the bucket walk does
  `lwz r8,4(r3)` to get the wide string it compares. So *every* resource carries its own key.

That is the whole trick. Find the buffer, subtract 152, read `+4`:

```
guest 0x312883F0   FMG buffer (relocated: +0x14 holds 0x31288418 = buffer+0x28)
guest 0x31286EC8   the only pointer to it in all of guest memory  ->  object = 0x31286E30
guest 0x31286E30   +0   = 0x01CA8E30   vtable
                   +4   = 0x30003440   -> UTF-16BE "regulation.fmg"     <-- THE KEY
                   +152 = 0x312883F0   buffer
                   +156 = 0x00000400   1024 bytes allocated for a 128-byte file
                   +164 = 0x01         dirty flag
```

`tools/memscan` grew a `-read`/`-base` mode for this. Guest memory maps at host **`0x300000000`**
(found by scanning for a string whose EBOOT vaddr we knew, `text:/Text/%s/regulation.fmg` at
`0x1881580`, and subtracting); RPCS3 mirrors the same pages at `0x400000000` and higher, so ignore
duplicate hits above `0x40000000` guest.

### The bucket walk — a positive control, and the collision check

The repository itself is reachable and readable, which lets us reproduce the client's own lookup by
hand and confirm every assumption at once.

```
repo   = *( *(0x1E1D810) + 24 )              ; 0x76FE84: lwz r11,0(r9); lwz r11,24(r11)
       = *( 0x30A00170 + 24 ) = 0x3567D540
repo+8  = 0x6D = 109                         ; bucket count (modulus)
repo+20 = 0x305E3130                         ; bucket array, stride 20 bytes
```

The hash, straight from `0xC169E8` — fold `A`–`Z` to lower case, then `h = c + h*137` over the
UTF-16 code units:

```python
def h(s):
    v = 0
    for ch in s:
        c = ord(ch)
        if 65 <= c <= 90: c += 32
        v = (c + v*137) & 0xffffffff
    return v
```

`h("regulation.fmg") % 109 = 11`, so bucket = `0x305E3130 + 11*20 = 0x305E320C`, whose
`{begin,end}` = `{0x3079FA30, 0x3079FA5C}` — 11 entries. Reading `+4` of each:

```
31d24af0  menu:/18.febnd.dcx              3569e2a0  dlc_data:/EzState/talk_m10_04_00_00.esd
33eab720  AS_1010_M.vpo                   356b8180  lotpf:/m10_04_00_00/enkei3_d
31286e30  regulation.fmg   <-- ours       3568e4a0  dlc_data:/ezstate/ai741000.esd
366c02b0  gibnd:/m10_04_00_00/...tpf.dcx  36baef30  mapbnd:/m10_04_00_00/m1525.flv.dcx
3128bce0  eventmaker:/EventMakerEx/...    36c3d9b0  hihkxbnd:/m10_04_00_00/h02_1000.hkx.dcx
                                          36d48db0  gibnd:/m10_04_00_00/m0368_gi_00.tpf.dcx
```

**Exactly one entry in the bucket is named `regulation.fmg`.** The other ten merely collide. That
retires the crash hazard for this particular push: there is no second `regulation.fmg` for the walk
to reach first, and the one it reaches has a 1024-byte buffer for our 128-byte payload.

The many other `regulation.fmg` strings in memory (guest `0x100Axxxx`, `0x108Cxxxx` — sixteen of
them) live in the **archive directory** region, not the repository. They are BND entry names for
other containers, which is what the earlier crash hit. They are not registry keys.

### The type gate also passes — no base-class walk needed

`0xC169E8` matches on name first, then calls `node->vtbl[0]()` and walks the returned type's parent
chain (`lwz r3,4(r9)`) against `key+4`. Our object's `vtbl[0]` is OPD `0x01D56608` → `0x76B4CC`, and
`0x76B4CC` is **byte-for-byte identical to `0x76B514`**, the function the applier calls to build the
key type. Both are `lwz r30,-29432(r2); lwz r9,-32728(r30); lwz r3,0(r9)` with the same lazy init via
`0x76A86C`. Same cached descriptor, so it matches on the first compare.

### Why the template string fooled us

`text:/Text/English/regulation.fmg` really is resident — at guest `0x30009002`, in the *same string
pool*, a few hundred bytes from the real key at `0x30003440`. It is the **load** path (formatted from
`0x1881580`), used to find the file in the virtual filesystem; the repository is then keyed on the
basename. Finding the string and concluding "our path form is correct" was the error: presence in
memory says a string exists, not that it is a key. The only thing that proves a key is walking the
container that keys on it.

## Route A is the live path — `repo+172` is NOT null (2026-08-07)

Open item 3 below asked whether `r16 = *(repo+172)` is non-null at runtime. **It is:**
`*(0x3567D5EC) = 0x3567D600`, an object whose `+4` names it **`title_patch:/regulation.bnd.dcx`** —
the patch regulation archive. So `0x76FF48` / `0x7705B8` send *every* entry to `0x76BB30`, and the
extension test at `0x770800` and the lookup at `0x770848` are **dead code on this build**.

`0x76BB30(archive, name, source)` is not merely an overlay register — it is a second, parallel copy
of the same two routes:

```
76bb50: r31 = *(r30-32596)          ; container global; if *r31 == 0 -> return, silently
76bb74: bl 0xDE50F8                 ; GetExtension(name)
76bb84: bl 0x184DF2C                ; wcscmp against an extension literal
76bb90: bne -> 0x76BC0C             ; other extension: 0xBD3478 / 0xBD3120 on *(r31)   [params]
76bb94: r11 = *( *(r30-32728) )     ; the SAME cached type as 0x76B514
76bba8: r29 = *( *(r30-32524) + 24 ); a repository, reached by the same *(*(g))+24 shape
76bbc0: bl 0xC169E8(r29, {name, type})
76bbd0: beq -> return               ; not found -> silent, harmless
76bbdc: cmpwi r5,1024 ; ble         ; payload size gate, same 1024 cap
```

So the FMG rule is unchanged in substance — **key by the registered name, cap 1024, no destination
size check** — it just arrives via `0x76BB30` instead of `0x770848`. And the param branch keys off a
different container, which is consistent with bare `OnlineEventParam.param` having worked.

This also means the mechanism FromSoftware actually shipped is an **archive overlay**: pushes are
staged into `title_patch:/regulation.bnd.dcx` keyed by BND entry name, and the applier's tail
(`0x770484: bl 0x75E158`, after `current_version` is written) is the reload that makes params take
effect. That is why a param push visibly moved the chest.

## OPEN — resolve before building a payload

1. ~~**Repository identity.**~~ **CLOSED** by the chest run (`tasks/majula-event-chest.md`): the
   applier's repo and the chest's threshold reader are the same repository. Independently confirmed
   here — `*(*(0x1E1D810)+24) = 0x3567D540` is the repository that actually holds the loaded
   `regulation.fmg`, verified by reproducing its hash and walking its bucket.
2. ~~**`path` spelling.**~~ **CLOSED.** Bare names are correct for both routes: `OnlineEventParam.param`
   armed the chest, and the FMG key is the bare `regulation.fmg`.
3. ~~**Route A.**~~ **CLOSED** — see above. `repo+172` is non-null, Route A is the only path that runs,
   and it *does* apply rather than merely register.
4. **Which repository Route A's `.fmg` branch searches.** `0x76BB30` uses `*(*(r30-32524)+24)`, a
   different global from the applier's `*(*(0x1E1D810))+24`. Almost certainly the same singleton, but
   the anchor for `lwz r30,-29432(r2)` was not pinned, so the pool slot could not be read. **A wrong
   guess here is silent, not fatal** — a missed lookup returns NULL and the entry is skipped. Pin the
   anchor by finding a slot that resolves `-32596`, `-32728`, `-32524` to plausible globals; the
   applier's anchor `0x1D1C2AC` is confirmed (`-32668 -> 0x1E1D810`, `-32660 -> L".fmg"`) and is
   *not* the same one.
5. **v1.00 not cross-checked** beyond confirming the `L"param:/"` and `L".fmg"` literals exist
   (`0x17FD600` / `0x17FD4C8`).

## CONFIRMED ON SCREEN — the obelisk reads what we send (2026-08-07)

With `path = regulation.fmg` the obelisk displayed `0x038B LANDED. THE PUSH REACHED THE FMG!`. Both
surfaces this message can reach are now proven end to end: the chest (params) and the obelisk (FMG).

It is a config field now, not a payload file — `DSO_OBELISK_TEXT`, with the server synthesising the
whole FMG in `internal/server/game/obelisk.go`. `TestObeliskFMGMatchesStockFile` builds the stock
line and compares against the real extracted file byte for byte, so every header field the builder
writes is pinned to something the client has actually accepted.

The one hazard the builder has to enforce: **489 characters.** The destination buffer for this
resource is 1024 bytes (`resource+156`, read live) and neither apply route compares the payload
against it — the `<= 1024` test in `0x76A0F0` and `0x76BB30` gates the *payload*, not the
destination. Oversize is refused rather than truncated, because a silently shortened message reads
exactly like a working one.

## The obelisk test, ready to run

`data/regpush/armed/regulationEnglish.fmg` is already built — 128 bytes, byte-identical to the stock
file except the string, which reads `0x038B LANDED. THE PUSH REACHED THE FMG!`. The live buffer at
`0x312883F0` matches the stock file word for word, so the payload is size-correct by construction.

```
DSO_REGULATION_PUSH_FILE=data/regpush/armed/regulationEnglish.fmg
DSO_REGULATION_PUSH_PATH=regulation.fmg
DSO_REGULATION_PUSH_VERSION_REQUIRED=11500
```

**Send exactly this one path.** The 1024-byte cap in both `0x76A0F0` and `0x76BB30` is a payload
gate, not a destination gate — a speculative name that *hits* is memory corruption. This one is safe
because the bucket walk above shows it resolves to a single 1024-byte record.

Read the result with `tools/memscan`, not with your eyes: Majula copies the string into its own
display state at map load, so the on-screen text can read "unchanged" on a complete success.

```
memscan.exe -proc rpcs3.exe -utf16be "THE PUSH REACHED THE FMG"
```

A hit at guest `0x312883F0`+0x28 is the apply. A hit only in the server's own memory is not.

## Why this reframes the chest

`OnlineEventParam` row 0 and `ItemLotParam2_SvrEvent[10045500]` are byte-identical across all ten
published calibrations, including two dated on event days — yet the event demonstrably ran. This
message explains that cleanly: **FromSoftware pushed whole param files at runtime over `0x038B`**,
so the weekly data never had to appear in the S3 calibration channel at all. Nothing needs to be
scheduled client-side, which is consistent with `start_at`/`end_at` being dead.

See `tasks/majula-event-chest.md` for the chest gate itself (`OnlineEventParam` row 0, `u16` at `+2`).
