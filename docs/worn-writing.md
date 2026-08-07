# The worn writing — the Majula obelisk

> "The letters are worn beyond recognition."

That is what the obelisk in Majula says when nothing is being announced, and it is what every
player has seen since the servers went down. It is not a placeholder: FromSoftware used this
stone as a live noticeboard, rewriting it three times in 2014 to announce the Lost Crowns
trilogy, and the worn-letters line is the resting state between announcements.

We can write to it again. See [the mechanism](#how-it-was-delivered) below.

## What it actually is

The obelisk's text is **string id 100 of the regulation FMG** — one small file inside the
regulation archive that exists to hold this single string and nothing else, in eleven
per-language copies (`regulationJapanese.fmg`, `regulationEnglish.fmg`, ...).

The English file is 128 bytes. The string occupies exactly 40 characters, which is the length of
the worn-letters line — so the file was sized around its own default.

It is byte-identical across all ten published calibrations, including two dated on event days.
The announcements never travelled through the calibration channel at all.

## The three announcements

Each teaser named the way in to the DLC it was announcing, not the DLC's own name.

| DLC | Released | The obelisk read | Way in |
|---|---|---|---|
| **Crown of the Sunken King** | 2014-07-22 | Seek the land of an ancient king,<br>in the Black Gulch, deep below | Black Gulch → Shulva |
| **Crown of the Old Iron King** | 2014-08-26 | Seek the land of an ancient king,<br>past the Iron Keep, upon a sea of flame | Iron Keep → Brume Tower |
| **Crown of the Ivory King** | 2014-09-30 | Seek the land of an ancient king,<br>sealed away, at the Shrine of Winter | Shrine of Winter → Eleum Loyce |

Every line opens the same way — *Seek the land of an ancient king* — and then gives a place. Two
lines, always.

### References

- **Crown of the Sunken King** — <https://www.youtube.com/watch?v=DiPh1WMObUU&t=12s>
- **Crown of the Old Iron King** — <https://imgur.com/mCuFiYD>,
  <https://youtu.be/06-7d_uM770?t=24>
- **Crown of the Ivory King** — <https://youtu.be/lA2_2nxymqg?t=1>

> **One thing to check.** In the notes these were transcribed from, the Sunken King and Old Iron
> King links sat against the opposite lines — Sunken King against *past the Iron Keep*, Old Iron
> King against *in the Black Gulch*. The table above pairs them by the in-game entrances instead,
> which are not in doubt: Shulva is entered from the Black Gulch with the Dragon Talon, and Brume
> Tower from the Iron Keep with the Scorching Iron Scepter. The captures are the way to settle it
> for certain and **have not been re-checked** — if one of them shows otherwise, the table is
> wrong and the notes were right.

## How it was delivered

Not through a patch, and not through the calibration download. Over **`0x038B
RegulationFileUpdatePushMessage`**, a live server push that replaces one whole file in the
running game — the same message that drove the weekly Majula event chest.

The client keys the resource as the bare name **`regulation.fmg`**. The applier memcpys the
payload straight into the buffer the file was loaded into and the new text is live without a
restart.

Full mechanism, and how the key was recovered from a running client, in
[`tasks/regulation-push-038b.md`](../tasks/regulation-push-038b.md).

## Writing to it

```
DSO_OBELISK_TEXT=Seek the land of an ancient king,\nin the Black Gulch, deep below
```

The server builds the whole FMG around the text — there is no file to edit. `\n` starts a new
line, since an env var cannot carry a real one.

Two constraints, both worth respecting:

- **489 characters.** The client's buffer for this resource is 1024 bytes and *neither* apply
  route compares the payload against it, so overrunning corrupts the client rather than failing.
  Oversize is refused rather than truncated.
- **Push before Majula loads.** The `.fmg` route writes into the loaded buffer, but Majula copies
  the string into its own display state at map load. Push while the player is already standing
  there and the stone will not change until they leave and come back. The default of sending at
  login is the right order.

Keeping to the original's shape — one opening line, one place, no more than about forty
characters a line — is what makes a new message read like it belongs on the stone.
