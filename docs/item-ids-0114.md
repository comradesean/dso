# Item ids available in calibration 0114

**Generated — do not edit by hand.** Regenerate with `python3 tools/gamedata/item_map.py <unpacked.bnd> -o docs/item-ids-0114.md`.

This is the menu for the Majula event chest. An `ItemLotParam2_SvrEvent` row's item field
at `+0x2C` takes one of these ids, and that row can be rewritten and pushed to a running
client over `0x038B` — see `tasks/majula-event-chest.md`.

## How this was built, and what it does not prove

Two independent sources, intersected:

- **The id space and English names** come from the game's own item database, extracted to
  `tools/gamedata/ds2_items_english.tsv`.
- **What this regulation defines** comes from reading ItemParam, WeaponParam, ArmorParam, RingParam out of the
  unpacked BND4.

An id in both columns is defined by the regulation the client is running, which is the
strongest statement available without putting it in a chest. It does **not** prove the item
is obtainable, that its icon and description exist, or that a lot referencing it behaves
sensibly. Only a live claim proves that, and so far exactly one id has been proven that
way: `60420000` Torch, which the chest handed over on 2026-08-06.

## Totals

| source | rows |
|---|---|
| `ItemParam` | 1268 |
| `WeaponParam` | 456 |
| `ArmorParam` | 498 |
| `RingParam` | 126 |
| **named ids defined by this regulation** | **1231** |
| named ids NOT found in those params (unverified) | 103 |
| ids defined but with no English name | 657 |

## Bands

| range | category | available |
|---|---|---|
| `1000000`–`6999999` | Weapons | 257 |
| `11000000`–`11999999` | Shields | 74 |
| `21000000`–`27999999` | Armor | 431 |
| `31000000`–`35999999` | Spells | 106 |
| `40000000`–`42999999` | Rings | 124 |
| `50000000`–`53999999` | Keys and quest items | 37 |
| `60000000`–`61999999` | Consumables | 118 |
| `62000000`–`62999999` | Multiplayer items | 17 |
| `63000000`–`63999999` | Gestures | 20 |
| `64000000`–`64999999` | Boss souls | 45 |

## Available ids


### Other

| id | name | defined in |
|---|---|---|
| `10` | No Item | ArmorParam, ItemParam, RingParam, WeaponParam |
| `20` | No Spell | ArmorParam |

### Weapons

| id | name | defined in |
|---|---|---|
| `1000000` | Dagger | ItemParam, WeaponParam |
| `1010000` | Bandit's Knife | ItemParam, WeaponParam |
| `1040000` | Mytha's Bent Blade | ItemParam, WeaponParam |
| `1050000` | Shadow Dagger | ItemParam, WeaponParam |
| `1060000` | Thief Dagger | ItemParam, WeaponParam |
| `1070000` | Broken Thief Sword | ItemParam, WeaponParam |
| `1100000` | Parrying Dagger | ItemParam, WeaponParam |
| `1110000` | Manikin Knife | ItemParam, WeaponParam |
| `1130000` | Royal Dirk | ItemParam, WeaponParam |
| `1140000` | Blue Dagger | ItemParam, WeaponParam |
| `1150000` | Umbral Dagger | ItemParam, WeaponParam |
| `1160000` | Retainer's Short Sword | ItemParam, WeaponParam |
| `1200000` | Broken Straight Sword | ItemParam, WeaponParam |
| `1210000` | Shortsword | ItemParam, WeaponParam |
| `1220000` | Longsword | ItemParam, WeaponParam |
| `1230000` | Broadsword | ItemParam, WeaponParam |
| `1240000` | Foot Soldier Sword | ItemParam, WeaponParam |
| `1250000` | Puzzling Stone Sword | ItemParam, WeaponParam |
| `1260000` | Possessed Armor Sword | ItemParam, WeaponParam |
| `1270000` | Varangian Sword | ItemParam, WeaponParam |
| `1280000` | Blue Flame | ItemParam, WeaponParam |
| `1290000` | Fume Sword | ItemParam, WeaponParam |
| `1320000` | Heide Knight Sword | ItemParam, WeaponParam |
| `1330000` | Red Rust Sword | ItemParam, WeaponParam |
| `1350000` | Black Dragon Sword | ItemParam, WeaponParam |
| `1360000` | Sun Sword | ItemParam, WeaponParam |
| `1370000` | Drakekeeper's Sword | ItemParam, WeaponParam |
| `1380000` | Ashen Warrior Sword | ItemParam, WeaponParam |
| `1390000` | Ivory Straight Sword | ItemParam, WeaponParam |
| `1400000` | Estoc | ItemParam, WeaponParam |
| `1410000` | Mail Breaker | ItemParam, WeaponParam |
| `1420000` | Chaos Rapier | ItemParam, WeaponParam |
| `1430000` | Spider's Silk | ItemParam, WeaponParam |
| `1440000` | Espada Ropera | ItemParam, WeaponParam |
| `1500000` | Rapier | ItemParam, WeaponParam |
| `1520000` | Black Scorpion Stinger | ItemParam, WeaponParam |
| `1530000` | Ricard's Rapier | ItemParam, WeaponParam |
| `1580000` | Ice Rapier | ItemParam, WeaponParam |
| `1600000` | Falchion | ItemParam, WeaponParam |
| `1610000` | Shotel | ItemParam, WeaponParam |
| `1620000` | Warped Sword | ItemParam, WeaponParam |
| `1630000` | Eleum Loyce | ItemParam, WeaponParam |
| `1640000` | Manikin Sabre | ItemParam, WeaponParam |
| `1650000` | Scimitar | ItemParam, WeaponParam |
| `1660000` | Red Rust Scimitar | ItemParam, WeaponParam |
| `1670000` | Spider Fang | ItemParam, WeaponParam |
| `1680000` | Melu Scimitar | ItemParam, WeaponParam |
| `1690000` | Monastery Scimitar | ItemParam, WeaponParam |
| `1700000` | Uchigatana | ItemParam, WeaponParam |
| `1710000` | Washing Pole | ItemParam, WeaponParam |
| `1720000` | Chaos Blade | ItemParam, WeaponParam |
| `1730000` | Blacksteel Katana | ItemParam, WeaponParam |
| `1740000` | Manslayer | ItemParam, WeaponParam |
| `1760000` | Darkdrift | ItemParam, WeaponParam |
| `1770000` | Berserker Blade | ItemParam, WeaponParam |
| `1790000` | Bewitched Alonne Sword | ItemParam, WeaponParam |
| `1800000` | Bastard Sword | ItemParam, WeaponParam |
| `1810000` | Flamberge | ItemParam, WeaponParam |
| `1820000` | Claymore | ItemParam, WeaponParam |
| `1830000` | Majestic Greatsword | ItemParam, WeaponParam |
| `1831000` | Majestic Greatsword | ItemParam, WeaponParam |
| `1850000` | Drangleic Sword | ItemParam, WeaponParam |
| `1860000` | Thorned Greatsword | ItemParam, WeaponParam |
| `1870000` | Bluemoon Greatsword | ItemParam, WeaponParam |
| `1871000` | Moonlight Greatsword | ItemParam, WeaponParam |
| `1880000` | Mastodon Greatsword | ItemParam, WeaponParam |
| `1900000` | Ruler's Sword | ItemParam, WeaponParam |
| `1910000` | Mirrah Greatsword | ItemParam, WeaponParam |
| `1911000` | Old Mirrah Greatsword | ItemParam, WeaponParam |
| `1920000` | Black Dragon Greatsword | ItemParam, WeaponParam |
| `1930000` | Black Knight Greatsword | ItemParam, WeaponParam |
| `1940000` | Royal Greatsword | ItemParam, WeaponParam |
| `1950000` | Old Knight Greatsword | ItemParam, WeaponParam |
| `1960000` | Defender Greatsword | ItemParam, WeaponParam |
| `1970000` | Watcher Greatsword | ItemParam, WeaponParam |
| `1980000` | Key to the Embedded | ItemParam, WeaponParam |
| `1990000` | Drakeblood Greatsword | ItemParam, WeaponParam |
| `1995000` | Loyce Greatsword | ItemParam, WeaponParam |
| `1996000` | Charred Loyce Greatsword | ItemParam, WeaponParam |
| `2000000` | Hand Axe | ItemParam, WeaponParam |
| `2010000` | Battle Axe | ItemParam, WeaponParam |
| `2020000` | Bandit Axe | ItemParam, WeaponParam |
| `2030000` | Infantry Axe | ItemParam, WeaponParam |
| `2070000` | Gyrm Axe | ItemParam, WeaponParam |
| `2080000` | Dragonslayer's Crescent Axe | ItemParam, WeaponParam |
| `2090000` | Butcher's Knife | ItemParam, WeaponParam |
| `2100000` | Silverblack Sickle | ItemParam, WeaponParam |
| `2200000` | Crescent Axe | ItemParam, WeaponParam |
| `2210000` | Greataxe | ItemParam, WeaponParam |
| `2220000` | Bandit Greataxe | ItemParam, WeaponParam |
| `2240000` | Lion Greataxe | ItemParam, WeaponParam |
| `2250000` | Giant Stone Axe | ItemParam, WeaponParam |
| `2260000` | Gyrm Greataxe | ItemParam, WeaponParam |
| `2290000` | Black Dragon Greataxe | ItemParam, WeaponParam |
| `2300000` | Black Knight Greataxe | ItemParam, WeaponParam |
| `2310000` | Drakekeeper's Greataxe | ItemParam, WeaponParam |
| `2400000` | Club | ItemParam, WeaponParam |
| `2410000` | Mace | ItemParam, WeaponParam |
| `2420000` | Morning Star | ItemParam, WeaponParam |
| `2430000` | Reinforced Club | ItemParam, WeaponParam |
| `2440000` | Craftsman's Hammer | ItemParam, WeaponParam |
| `2470000` | Mace of the Insolent | ItemParam, WeaponParam |
| `2500000` | Handmaid's Ladle | ItemParam, WeaponParam |
| `2520000` | Blacksmith's Hammer | ItemParam, WeaponParam |
| `2530000` | Black Dragon Warpick | ItemParam, WeaponParam |
| `2540000` | Aldia Hammer | ItemParam, WeaponParam |
| `2560000` | Barbed Club | ItemParam, WeaponParam |
| `2600000` | Large Club | ItemParam, WeaponParam |
| `2610000` | Pickaxe | ItemParam, WeaponParam |
| `2620000` | Great Club | ItemParam, WeaponParam |
| `2630000` | Gyrm Great Hammer | ItemParam, WeaponParam |
| `2660000` | Iron King Hammer | ItemParam, WeaponParam |
| `2670000` | Malformed Skull | ItemParam, WeaponParam |
| `2680000` | Dragon Tooth | ItemParam, WeaponParam |
| `2690000` | Giant Warrior Club | ItemParam, WeaponParam |
| `2700000` | Malformed Shell | ItemParam, WeaponParam |
| `2710000` | Demon's Great Hammer | ItemParam, WeaponParam |
| `2720000` | Archdrake Mace | ItemParam, WeaponParam |
| `2730000` | Old Knight Hammer | ItemParam, WeaponParam |
| `2740000` | Drakekeeper's Great Hammer | ItemParam, WeaponParam |
| `2750000` | Sacred Chime Hammer | ItemParam, WeaponParam |
| `2760000` | Sanctum Mace | ItemParam, WeaponParam |
| `2800000` | Spear | ItemParam, WeaponParam |
| `2810000` | Winged Spear | ItemParam, WeaponParam |
| `2820000` | Pike | ItemParam, WeaponParam |
| `2830000` | Partizan | ItemParam, WeaponParam |
| `2840000` | Stone Soldier Spear | ItemParam, WeaponParam |
| `2850000` | Spitfire Spear | ItemParam, WeaponParam |
| `2855000` | Yorgh's Spear | ItemParam, WeaponParam |
| `2860000` | Silverblack Spear | ItemParam, WeaponParam |
| `2870000` | Heide Spear | ItemParam, WeaponParam |
| `2880000` | Pate's Spear | ItemParam, WeaponParam |
| `2890000` | Channeler's Trident | ItemParam, WeaponParam |
| `2895000` | Gargoyle Bident | ItemParam, WeaponParam |
| `2896000` | Dragonslayer Spear | ItemParam, WeaponParam |
| `2900000` | Heide Lance | ItemParam, WeaponParam |
| `2920000` | Heide Greatlance | ItemParam, WeaponParam |
| `2930000` | Grand Lance | ItemParam, WeaponParam |
| `2940000` | Chariot Lance | ItemParam, WeaponParam |
| `2950000` | Rampart Golem Lance | ItemParam, WeaponParam |
| `2960000` | Smelter Hammer | ItemParam, WeaponParam |
| `3000000` | Great Scythe | ItemParam, WeaponParam |
| `3010000` | Great Machete | ItemParam, WeaponParam |
| `3020000` | Full Moon Sickle | ItemParam, WeaponParam |
| `3040000` | Crescent Sickle | ItemParam, WeaponParam |
| `3050000` | Scythe of Nahr Alma | ItemParam, WeaponParam |
| `3060000` | Bone Scythe | ItemParam, WeaponParam |
| `3070000` | Scythe of Want | ItemParam, WeaponParam |
| `3200000` | Lucerne | ItemParam, WeaponParam |
| `3210000` | Scythe | ItemParam, WeaponParam |
| `3220000` | Halberd | ItemParam, WeaponParam |
| `3240000` | Helix Halberd | ItemParam, WeaponParam |
| `3250000` | Santier's Spear | ItemParam, WeaponParam |
| `3251000` | Santier's Spear | ItemParam, WeaponParam |
| `3270000` | Mastodon Halberd | ItemParam, WeaponParam |
| `3280000` | Blue Knight's Halberd | ItemParam, WeaponParam |
| `3290000` | Dragonrider's Halberd | ItemParam, WeaponParam |
| `3300000` | Black Knight Halberd | ItemParam, WeaponParam |
| `3310000` | Syan's Halberd | ItemParam, WeaponParam |
| `3320000` | Roaring Halberd | ItemParam, WeaponParam |
| `3330000` | Old Knight Halberd | ItemParam, WeaponParam |
| `3340000` | Old Knight Pike | ItemParam, WeaponParam |
| `3350000` | Drakekeeper's Warpick | ItemParam, WeaponParam |
| `3370000` | Wrathful Axe | ItemParam, WeaponParam |
| `3410000` | Claws | ItemParam, WeaponParam |
| `3420000` | Malformed Claws | ItemParam, WeaponParam |
| `3430000` | Manikin Claws | ItemParam, WeaponParam |
| `3440000` | Work Hook | ItemParam, WeaponParam |
| `3500000` | Caestus | ItemParam, WeaponParam |
| `3530000` | Bone Fist | ItemParam, WeaponParam |
| `3600000` | Whip | ItemParam, WeaponParam |
| `3610000` | Notched Whip | ItemParam, WeaponParam |
| `3620000` | Bloodied Whip | ItemParam, WeaponParam |
| `3630000` | Spotted Whip | ItemParam, WeaponParam |
| `3660000` | Old Whip | ItemParam, WeaponParam |
| `3800000` | Sorcerer's Staff | ItemParam, WeaponParam |
| `3810000` | Staff of Amana | ItemParam, WeaponParam |
| `3820000` | Witchtree Branch | ItemParam, WeaponParam |
| `3830000` | Lizard Staff | ItemParam, WeaponParam |
| `3850000` | Olenford's Staff | ItemParam, WeaponParam |
| `3860000` | Archdrake Staff | ItemParam, WeaponParam |
| `3870000` | Bat Staff | ItemParam, WeaponParam |
| `3880000` | Bone Staff | ItemParam, WeaponParam |
| `3890000` | Staff of Wisdom | ItemParam, WeaponParam |
| `3900000` | Sunset Staff | ItemParam, WeaponParam |
| `3910000` | Pilgrim's Spontoon | ItemParam, WeaponParam |
| `3930000` | Azal's Staff | ItemParam, WeaponParam |
| `3940000` | Retainer Staff | ItemParam, WeaponParam |
| `4010000` | Cleric's Sacred Chime | ItemParam, WeaponParam |
| `4020000` | Witchtree Bellvine | ItemParam, WeaponParam |
| `4030000` | Priest's Chime | ItemParam, WeaponParam |
| `4040000` | Dragon Chime | ItemParam, WeaponParam |
| `4050000` | Chime of Want | ItemParam, WeaponParam |
| `4060000` | Archdrake Chime | ItemParam, WeaponParam |
| `4080000` | Idol's Chime | ItemParam, WeaponParam |
| `4090000` | Caitha's Chime | ItemParam, WeaponParam |
| `4100000` | Protective Chime | ItemParam, WeaponParam |
| `4110000` | Disc Chime | ItemParam, WeaponParam |
| `4120000` | Chime of Screams | ItemParam, WeaponParam |
| `4150000` | Black Witch's Staff | ItemParam, WeaponParam |
| `4200000` | Short Bow | ItemParam, WeaponParam |
| `4210000` | Long Bow | ItemParam, WeaponParam |
| `4220000` | Composite Bow | ItemParam, WeaponParam |
| `4230000` | Sea Bow | ItemParam, WeaponParam |
| `4240000` | Dragonrider Bow | ItemParam, WeaponParam |
| `4270000` | Bell Keeper Bow | ItemParam, WeaponParam |
| `4280000` | Bow of Want | ItemParam, WeaponParam |
| `4290000` | Hunter's Blackbow | ItemParam, WeaponParam |
| `4400000` | Alonne Greatbow | ItemParam, WeaponParam |
| `4420000` | Dragonslayer Greatbow | ItemParam, WeaponParam |
| `4430000` | Possessed Armor Greatbow | ItemParam, WeaponParam |
| `4440000` | Twin-headed Greatbow | ItemParam, WeaponParam |
| `4600000` | Light Crossbow | ItemParam, WeaponParam |
| `4610000` | Heavy Crossbow | ItemParam, WeaponParam |
| `4630000` | Shield Crossbow | ItemParam, WeaponParam |
| `4660000` | Avelyn | ItemParam, WeaponParam |
| `4670000` | Sanctum Crossbow | ItemParam, WeaponParam |
| `4680000` | Sanctum Repeating Crossbow | ItemParam, WeaponParam |
| `5000000` | Murakumo | ItemParam, WeaponParam |
| `5010000` | Arced Sword | ItemParam, WeaponParam |
| `5040000` | Curved Dragon Greatsword | ItemParam, WeaponParam |
| `5050000` | Curved Nil Greatsword | ItemParam, WeaponParam |
| `5200000` | Zweihander | ItemParam, WeaponParam |
| `5210000` | Greatsword | ItemParam, WeaponParam |
| `5220000` | Smelter Sword | ItemParam, WeaponParam |
| `5225000` | Aged Smelter Sword | ItemParam, WeaponParam |
| `5230000` | Drakewing Ultra Greatsword | ItemParam, WeaponParam |
| `5240000` | King's Ultra Greatsword | ItemParam, WeaponParam |
| `5250000` | Fume Ultra Greatsword | ItemParam, WeaponParam |
| `5255000` | Ivory King Ultra Greatsword | ItemParam, WeaponParam |
| `5270000` | Pursuer's Ultra Greatsword | ItemParam, WeaponParam |
| `5275000` | Drakekeeper's Ultra Greatsword | ItemParam, WeaponParam |
| `5280000` | Crypt Blacksword | ItemParam, WeaponParam |
| `5285000` | Old Knight Ultra Greatsword | ItemParam, WeaponParam |
| `5290000` | Black Knight Ultra Greatsword | ItemParam, WeaponParam |
| `5295000` | Lost Sinner's Sword | ItemParam, WeaponParam |
| `5310000` | Stone Twinblade | ItemParam, WeaponParam |
| `5330000` | Dragonrider Twinblade | ItemParam, WeaponParam |
| `5340000` | Twinblade | ItemParam, WeaponParam |
| `5350000` | Red Iron Twinblade | ItemParam, WeaponParam |
| `5360000` | Curved Twinblade | ItemParam, WeaponParam |
| `5370000` | Sorcerer's Twinblade | ItemParam, WeaponParam |
| `5400000` | Pyromancy Flame | ItemParam, WeaponParam |
| `5410000` | Dark Pyromancy Flame | ItemParam, WeaponParam |
| `5500000` | Black Flamestone Dagger | ItemParam, WeaponParam |
| `5510000` | Yellow Quartz Longsword | ItemParam, WeaponParam |
| `5520000` | Bound Hand Axe | ItemParam, WeaponParam |
| `5530000` | Homunculus Mace | ItemParam, WeaponParam |
| `5540000` | Transgressor's Staff | ItemParam, WeaponParam |
| `5600000` | Longsword | ItemParam, WeaponParam |
| `5610000` | Murakumo | ItemParam, WeaponParam |
| `5620000` | Blacksteel Katana | ItemParam, WeaponParam |
| `5630000` | Rapier | ItemParam, WeaponParam |
| `5640000` | Greataxe | ItemParam, WeaponParam |
| `5650000` | Great Club | ItemParam, WeaponParam |
| `5660000` | Caestus | ItemParam, WeaponParam |
| `6100000` | Binoculars | ItemParam, WeaponParam |

### Shields

| id | name | defined in |
|---|---|---|
| `11000000` | Buckler | ItemParam, WeaponParam |
| `11005000` | Benhart's Parma | ItemParam, WeaponParam |
| `11010000` | Small Leather Shield | ItemParam, WeaponParam |
| `11020000` | Iron Parma | ItemParam, WeaponParam |
| `11030000` | Foot Soldier Shield | ItemParam, WeaponParam |
| `11040000` | Target Shield | ItemParam, WeaponParam |
| `11050000` | Golden Falcon Shield | ItemParam, WeaponParam |
| `11070000` | Manikin Shield | ItemParam, WeaponParam |
| `11080000` | Llewellyn Shield | ItemParam, WeaponParam |
| `11091000` | Crimson Parma | ItemParam, WeaponParam |
| `11110000` | Cleric's Parma | ItemParam, WeaponParam |
| `11120000` | Cleric's Small Shield | ItemParam, WeaponParam |
| `11130000` | Magic Shield | ItemParam, WeaponParam |
| `11140000` | Cursed Bone Shield | ItemParam, WeaponParam |
| `11150000` | Sanctum Shield | ItemParam, WeaponParam |
| `11160000` | Varangian Shield | ItemParam, WeaponParam |
| `11185000` | Watcher's Shield | ItemParam, WeaponParam |
| `11200000` | Large Leather Shield | ItemParam, WeaponParam |
| `11210000` | Blue Wooden Shield | ItemParam, WeaponParam |
| `11220000` | Silver Eagle Kite Shield | ItemParam, WeaponParam |
| `11230000` | Drangleic Shield | ItemParam, WeaponParam |
| `11240000` | Lion Clan Shield | ItemParam, WeaponParam |
| `11250000` | Archdrake Shield | ItemParam, WeaponParam |
| `11260000` | King's Shield | ItemParam, WeaponParam |
| `11270000` | Mirrah Shield | ItemParam, WeaponParam |
| `11280000` | Old Knight's Shield | ItemParam, WeaponParam |
| `11290000` | Loyce Shield | ItemParam, WeaponParam |
| `11295000` | Charred Loyce Shield | ItemParam, WeaponParam |
| `11300000` | Spirit Tree Shield | ItemParam, WeaponParam |
| `11310000` | Golden Wing Shield | ItemParam, WeaponParam |
| `11320000` | Vessel Shield | ItemParam, WeaponParam |
| `11330000` | Shield of the Insolent | ItemParam, WeaponParam |
| `11350000` | Silverblack Shield | ItemParam, WeaponParam |
| `11360000` | Stone Parma | ItemParam, WeaponParam |
| `11370000` | Grand Spirit Tree Shield | ItemParam, WeaponParam |
| `11380000` | Moon Butterfly Shield | ItemParam, WeaponParam |
| `11390000` | Slumbering Dragon Shield | ItemParam, WeaponParam |
| `11395000` | Chaos Shield | ItemParam, WeaponParam |
| `11400000` | Wooden Shield | ItemParam, WeaponParam |
| `11420000` | Hollow Soldier Shield | ItemParam, WeaponParam |
| `11430000` | Royal Kite Shield | ItemParam, WeaponParam |
| `11450000` | Red Rust Shield | ItemParam, WeaponParam |
| `11455000` | Rampart Golem Shield | ItemParam, WeaponParam |
| `11470000` | Bell Keeper Shield | ItemParam, WeaponParam |
| `11475000` | Defender's Shield | ItemParam, WeaponParam |
| `11480000` | Black Dragon Shield | ItemParam, WeaponParam |
| `11485000` | Drakekeeper's Shield | ItemParam, WeaponParam |
| `11490000` | Porcine Shield | ItemParam, WeaponParam |
| `11495000` | Bone Shield | ItemParam, WeaponParam |
| `11500000` | Twin Dragon Greatshield | ItemParam, WeaponParam |
| `11510000` | Tower Shield | ItemParam, WeaponParam |
| `11530000` | Orma's Greatshield | ItemParam, WeaponParam |
| `11540000` | Reeve's Greatshield | ItemParam, WeaponParam |
| `11550000` | King's Mirror | ItemParam, WeaponParam |
| `11560000` | Dragonrider Greatshield | ItemParam, WeaponParam |
| `11570000` | Mastodon Greatshield | ItemParam, WeaponParam |
| `11590000` | Havel's Greatshield | ItemParam, WeaponParam |
| `11600000` | Gyrm Greatshield | ItemParam, WeaponParam |
| `11610000` | Pursuer's Greatshield | ItemParam, WeaponParam |
| `11620000` | Pate's Shield | ItemParam, WeaponParam |
| `11630000` | Old Knight Greatshield | ItemParam, WeaponParam |
| `11640000` | Drakekeeper's Greatshield | ItemParam, WeaponParam |
| `11650000` | Greatshield of Glory | ItemParam, WeaponParam |
| `11700000` | Phoenix Parma | ItemParam, WeaponParam |
| `11710000` | Sunlight Parma | ItemParam, WeaponParam |
| `11720000` | Watchdragon Parma | ItemParam, WeaponParam |
| `11730000` | Blossom Kite Shield | ItemParam, WeaponParam |
| `11740000` | Rebel's Greatshield | ItemParam, WeaponParam |
| `11750000` | Wicked Eye Greatshield | ItemParam, WeaponParam |
| `11800000` | Black Flamestone Parma | ItemParam, WeaponParam |
| `11810000` | Yellow Quartz Shield | ItemParam, WeaponParam |
| `11820000` | Bound Wooden Shield | ItemParam, WeaponParam |
| `11830000` | Homunculus Wooden Shield | ItemParam, WeaponParam |
| `11840000` | Transgressor's Leather Shield | ItemParam, WeaponParam |

### Armor

| id | name | defined in |
|---|---|---|
| `21010100` | Pate's Helm | ItemParam |
| `21010101` | Pate's Armor | ItemParam |
| `21010102` | Pate's Gloves | ItemParam |
| `21010103` | Pate's Trousers | ItemParam |
| `21020100` | Thief Mask | ItemParam |
| `21020101` | Black Leather Armor | ItemParam |
| `21020102` | Black Leather Gloves | ItemParam |
| `21020103` | Black Leather Boots | ItemParam |
| `21030100` | Wanderer Hood | ItemParam |
| `21030101` | Wanderer Coat | ItemParam |
| `21030102` | Wanderer Manchettes | ItemParam |
| `21030103` | Wanderer Boots | ItemParam |
| `21040100` | Hunter's Hat | ItemParam |
| `21040101` | Leather Armor | ItemParam |
| `21040102` | Leather Gloves | ItemParam |
| `21040103` | Leather Boots | ItemParam |
| `21050100` | Knight Helm | ItemParam |
| `21050101` | Knight Armor | ItemParam |
| `21050102` | Knight Gauntlets | ItemParam |
| `21050103` | Knight Leggings | ItemParam |
| `21060100` | Elite Knight Helm | ItemParam |
| `21060101` | Elite Knight Armor | ItemParam |
| `21060102` | Elite Knight Gloves | ItemParam |
| `21060103` | Elite Knight Leggings | ItemParam |
| `21070100` | Tattered Cloth Hood | ItemParam |
| `21070101` | Tattered Cloth Robe | ItemParam |
| `21070102` | Tattered Cloth Manchettes | ItemParam |
| `21070103` | Heavy Boots | ItemParam |
| `21080100` | Brigand Hood | ItemParam |
| `21080101` | Brigand Armor | ItemParam |
| `21080102` | Brigand Gauntlets | ItemParam |
| `21080103` | Brigand Trousers | ItemParam |
| `21100100` | Imported Hood | ItemParam |
| `21100101` | Imported Tunic | ItemParam |
| `21100102` | Imported Manchettes | ItemParam |
| `21100103` | Imported Trousers | ItemParam |
| `21140100` | Traveling Merchant Hat | ItemParam |
| `21140101` | Traveling Merchant Coat | ItemParam |
| `21140102` | Traveling Merchant Gloves | ItemParam |
| `21140103` | Traveling Merchant Boots | ItemParam |
| `21160100` | Havel's Helm | ItemParam |
| `21160101` | Havel's Armor | ItemParam |
| `21160102` | Havel's Gauntlets | ItemParam |
| `21160103` | Havel's Leggings | ItemParam |
| `21210100` | Jester's Cap | ItemParam |
| `21210101` | Jester's Robes | ItemParam |
| `21210102` | Jester's Gloves | ItemParam |
| `21210103` | Jester's Tights | ItemParam |
| `21230100` | Moon Hat | ItemParam |
| `21230101` | Astrologist's Robe | ItemParam |
| `21230102` | Astrologist's Gauntlets | ItemParam |
| `21230103` | Astrologist's Bottoms | ItemParam |
| `21320100` | Faraam Helm | ItemParam |
| `21320101` | Faraam Armor | ItemParam |
| `21320102` | Faraam Gauntlets | ItemParam |
| `21320103` | Faraam Boots | ItemParam |
| `21330100` | Black Dragon Helm | ItemParam |
| `21330101` | Black Dragon Armor | ItemParam |
| `21330102` | Black Dragon Gauntlets | ItemParam |
| `21330103` | Black Dragon Leggings | ItemParam |
| `21340100` | Xanthous Crown | ItemParam |
| `21340101` | Xanthous Overcoat | ItemParam |
| `21340102` | Xanthous Gloves | ItemParam |
| `21340103` | Xanthous Waistcloth | ItemParam |
| `21350100` | Mask of Judgment | ItemParam |
| `21350101` | Robe of Judgment | ItemParam |
| `21350102` | Manchettes of Judgment | ItemParam |
| `21350103` | Tights of Judgment | ItemParam |
| `21360100` | Helm of Aurous | ItemParam |
| `21360101` | Armor of Aurous | ItemParam |
| `21360102` | Gauntlets of Aurous | ItemParam |
| `21360103` | Leggings of Aurous | ItemParam |
| `21361100` | Helm of Aurous | ItemParam |
| `21361101` | Armor of Aurous | ItemParam |
| `21361102` | Gauntlets of Aurous | ItemParam |
| `21361103` | Leggings of Aurous | ItemParam |
| `21370100` | Monastery Headcloth | ItemParam |
| `21370101` | Monastery Longshirt | ItemParam |
| `21370102` | Monastery Long Gloves | ItemParam |
| `21370103` | Monastery Skirt | ItemParam |
| `21390100` | Dingy Hood | ItemParam |
| `21390101` | Dingy Robe | ItemParam |
| `21390102` | Dingy Cuffs | ItemParam |
| `21390103` | Blood-Stained Skirt | ItemParam |
| `21430100` | Durgo's Hat | ItemParam |
| `21440102` | Engraved Gauntlets | ItemParam |
| `21460103` | Flying Feline Boots | ItemParam |
| `21470100` | Moon Butterfly Hat | ItemParam |
| `21470101` | Moon Butterfly Wings | ItemParam |
| `21470102` | Moon Butterfly Cuffs | ItemParam |
| `21470103` | Moon Butterfly Skirt | ItemParam |
| `21480100` | Catarina Helm | ItemParam |
| `21480101` | Catarina Armor | ItemParam |
| `21480102` | Catarina Gauntlets | ItemParam |
| `21480103` | Catarina Leggings | ItemParam |
| `21490100` | Alva Helm | ItemParam |
| `21490101` | Alva Armor | ItemParam |
| `21490102` | Alva Gauntlets | ItemParam |
| `21490103` | Alva Leggings | ItemParam |
| `21500100` | Black Witch Veil | ItemParam |
| `21500101` | Black Witch Robe | ItemParam |
| `21500102` | Black Witch Gloves | ItemParam |
| `21500103` | Black Witch Trousers | ItemParam |
| `21501100` | Black Witch Hat | ItemParam |
| `21502100` | Black Witch Domino Mask | ItemParam |
| `21600100` | Drakeblood Helm | ItemParam |
| `21600101` | Drakeblood Armor | ItemParam |
| `21600102` | Drakeblood Gauntlets | ItemParam |
| `21600103` | Drakeblood Leggings | ItemParam |
| `21610100` | Northwarder Hood | ItemParam |
| `21610101` | Northwarder Robe | ItemParam |
| `21610102` | Northwarder Manchettes | ItemParam |
| `21610103` | Northwarder Trousers | ItemParam |
| `21630100` | Crown of the Old Iron King | ItemParam |
| `21640100` | Crown of the Ivory King | ItemParam |
| `21650100` | Crown of the Sunken King | ItemParam |
| `21660100` | Old Bell Helm | ItemParam |
| `21670100` | Hollow Skin | ItemParam |
| `21680100` | Pharros Mask | ItemParam |
| `21690103` | Flower Skirt | ItemParam |
| `21700100` | Minotaur Helm | ItemParam |
| `21710100` | Symbol of Avarice | ItemParam |
| `22020100` | Hollow Soldier Helm | ItemParam |
| `22020101` | Hollow Soldier Armor | ItemParam |
| `22020102` | Hollow Soldier Gauntlets | ItemParam |
| `22020103` | Hollow Soldier Leggings | ItemParam |
| `22021100` | Royal Soldier Helm | ItemParam |
| `22021101` | Royal Soldier Armor | ItemParam |
| `22021102` | Royal Soldier Gauntlets | ItemParam |
| `22021103` | Royal Soldier Leggings | ItemParam |
| `22030100` | Hollow Infantry Helm | ItemParam |
| `22030101` | Hollow Infantry Armor | ItemParam |
| `22030102` | Hollow Infantry Gloves | ItemParam |
| `22030103` | Hollow Infantry Boots | ItemParam |
| `22031100` | Infantry Helm | ItemParam |
| `22031101` | Infantry Armor | ItemParam |
| `22031102` | Infantry Gloves | ItemParam |
| `22031103` | Infantry Boots | ItemParam |
| `22060100` | White Priest Headpiece | ItemParam |
| `22060101` | White Priest Robe | ItemParam |
| `22060102` | White Priest Gloves | ItemParam |
| `22060103` | White Priest Skirt | ItemParam |
| `22062100` | Priestess Headpiece | ItemParam |
| `22062101` | Priestess Robe | ItemParam |
| `22062102` | Priestess Gloves | ItemParam |
| `22062103` | Priestess Skirt | ItemParam |
| `22080100` | Rogue Hood | ItemParam |
| `22080101` | Rogue Armor | ItemParam |
| `22080102` | Rogue Gauntlets | ItemParam |
| `22080103` | Rogue Leggings | ItemParam |
| `22110100` | Spiked Bandit Helm | ItemParam |
| `22110101` | Bandit Armor | ItemParam |
| `22110102` | Bandit Gauntlets | ItemParam |
| `22110103` | Bandit Boots | ItemParam |
| `22130100` | Varangian Helm | ItemParam |
| `22130101` | Varangian Armor | ItemParam |
| `22130102` | Varangian Cuffs | ItemParam |
| `22130103` | Varangian Leggings | ItemParam |
| `22180100` | Black Hollow Mage Hood | ItemParam |
| `22180101` | Black Hollow Mage Robe | ItemParam |
| `22182100` | White Hollow Mage Hood | ItemParam |
| `22182101` | White Hollow Mage Robe | ItemParam |
| `22190101` | Lion Mage Robe | ItemParam |
| `22190102` | Lion Mage Cuffs | ItemParam |
| `22190103` | Lion Mage Skirt | ItemParam |
| `22220100` | Steel Helm | ItemParam |
| `22220101` | Steel Armor | ItemParam |
| `22220102` | Steel Gauntlets | ItemParam |
| `22220103` | Steel Leggings | ItemParam |
| `22230100` | Shadow Mask | ItemParam |
| `22230101` | Shadow Top | ItemParam |
| `22230102` | Shadow Gauntlets | ItemParam |
| `22230103` | Shadow Leggings | ItemParam |
| `22240100` | Manikin Mask | ItemParam |
| `22240101` | Manikin Top | ItemParam |
| `22240102` | Manikin Gloves | ItemParam |
| `22240103` | Manikin Boots | ItemParam |
| `22270100` | Prisoner's Hood | ItemParam |
| `22270101` | Prisoner's Tatters | ItemParam |
| `22270102` | Prisoner's Gloves | ItemParam |
| `22270103` | Prisoner's Waistcloth | ItemParam |
| `22271100` | Prisoner's Hood | ItemParam |
| `22271101` | Prisoner's Tatters | ItemParam |
| `22310100` | Archdrake Helm | ItemParam |
| `22310101` | Archdrake Robes | ItemParam |
| `22310102` | Archdrake Gloves | ItemParam |
| `22310103` | Archdrake Boots | ItemParam |
| `22340100` | Gyrm Helm | ItemParam |
| `22340101` | Gyrm Armor | ItemParam |
| `22340102` | Gyrm Gloves | ItemParam |
| `22340103` | Gyrm Boots | ItemParam |
| `22350100` | Gyrm Warrior Helm | ItemParam |
| `22350101` | Gyrm Warrior Armor | ItemParam |
| `22350102` | Gyrm Warrior Gloves | ItemParam |
| `22350103` | Gyrm Warrior Boots | ItemParam |
| `22351100` | Gyrm Warrior Greathelm | ItemParam |
| `22360100` | Dark Mask | ItemParam |
| `22360101` | Dark Armor | ItemParam |
| `22360102` | Dark Gauntlets | ItemParam |
| `22360103` | Dark Leggings | ItemParam |
| `22370100` | Warlock Mask | ItemParam |
| `22460100` | Tseldora Cap | ItemParam |
| `22460101` | Tseldora Robe | ItemParam |
| `22460102` | Tseldora Manchettes | ItemParam |
| `22460103` | Tseldora Trousers | ItemParam |
| `22480100` | Peasant Hat | ItemParam |
| `22480101` | Peasant Attire | ItemParam |
| `22480102` | Peasant Long Gloves | ItemParam |
| `22480103` | Peasant Trousers | ItemParam |
| `22510100` | Ironclad Helm | ItemParam |
| `22510101` | Ironclad Armor | ItemParam |
| `22510102` | Ironclad Gauntlets | ItemParam |
| `22510103` | Ironclad Leggings | ItemParam |
| `22512100` | Old Ironclad Helm | ItemParam |
| `22512101` | Old Ironclad Armor | ItemParam |
| `22512102` | Old Ironclad Gauntlets | ItemParam |
| `22512103` | Old Ironclad Leggings | ItemParam |
| `22520100` | Royal Swordsman Helm | ItemParam |
| `22520101` | Royal Swordsman Armor | ItemParam |
| `22520102` | Royal Swordsman Gloves | ItemParam |
| `22520103` | Royal Swordsman Leggings | ItemParam |
| `22530100` | Syan's Helm | ItemParam |
| `22530101` | Syan's Armor | ItemParam |
| `22530102` | Syan's Gauntlets | ItemParam |
| `22530103` | Syan's Leggings | ItemParam |
| `22540100` | Bone Crown | ItemParam |
| `22540101` | Bone King Robe | ItemParam |
| `22540102` | Bone King Cuffs | ItemParam |
| `22540103` | Bone King Skirt | ItemParam |
| `23010100` | Heide Knight Greathelm | ItemParam |
| `23010101` | Heide Knight Chainmail | ItemParam |
| `23010102` | Heide Knight Gauntlets | ItemParam |
| `23010103` | Heide Knight Leggings | ItemParam |
| `23011100` | Heide Knight Iron Mask | ItemParam |
| `23040101` | Singer's Dress | ItemParam |
| `23050100` | Smelter Demon Helm | ItemParam |
| `23050101` | Smelter Demon Armor | ItemParam |
| `23050102` | Smelter Demon Gauntlets | ItemParam |
| `23050103` | Smelter Demon Leggings | ItemParam |
| `23060100` | Alonne Captain Helm | ItemParam |
| `23060101` | Alonne Captain Armor | ItemParam |
| `23061100` | Alonne Knight Helm | ItemParam |
| `23061101` | Alonne Knight Armor | ItemParam |
| `23061102` | Alonne Knight Gauntlets | ItemParam |
| `23061103` | Alonne Knight Leggings | ItemParam |
| `23070100` | Vengarl's Helm | ItemParam |
| `23070101` | Vengarl's Armor | ItemParam |
| `23070102` | Vengarl's Gloves | ItemParam |
| `23070103` | Vengarl's Boots | ItemParam |
| `23080101` | Lion Warrior Cape | ItemParam |
| `23080102` | Lion Warrior Cuffs | ItemParam |
| `23080103` | Lion Warrior Skirt | ItemParam |
| `23081100` | Lion Warrior Helm | ItemParam |
| `23081101` | Red Lion Warrior Cape | ItemParam |
| `23120100` | Grave Warden Mask | ItemParam |
| `23120101` | Grave Warden Top | ItemParam |
| `23120102` | Grave Warden Cuffs | ItemParam |
| `23120103` | Grave Warden Bottoms | ItemParam |
| `23130100` | Falconer Helm | ItemParam |
| `23130101` | Falconer Armor | ItemParam |
| `23130102` | Falconer Gloves | ItemParam |
| `23130103` | Falconer Boots | ItemParam |
| `23140100` | Rusted Mastodon Helm | ItemParam |
| `23140101` | Rusted Mastodon Armor | ItemParam |
| `23140102` | Rusted Mastodon Gauntlets | ItemParam |
| `23140103` | Rusted Mastodon Leggings | ItemParam |
| `23150100` | Mastodon Helm | ItemParam |
| `23150101` | Mastodon Armor | ItemParam |
| `23150102` | Mastodon Gauntlets | ItemParam |
| `23150103` | Mastodon Leggings | ItemParam |
| `23160100` | Desert Sorceress Hood | ItemParam |
| `23160101` | Desert Sorceress Top | ItemParam |
| `23160102` | Desert Sorceress Gloves | ItemParam |
| `23160103` | Desert Sorceress Skirt | ItemParam |
| `23170100` | Dragon Acolyte Mask | ItemParam |
| `23170101` | Dragon Acolyte Robe | ItemParam |
| `23170102` | Dragon Acolyte Gloves | ItemParam |
| `23170103` | Dragon Acolyte Boots | ItemParam |
| `23171100` | Dragon Sage Hood | ItemParam |
| `23250100` | Ruin Helm | ItemParam |
| `23250101` | Ruin Armor | ItemParam |
| `23250102` | Ruin Gauntlets | ItemParam |
| `23250103` | Ruin Leggings | ItemParam |
| `23300100` | Old Knight Helm | ItemParam |
| `23300101` | Old Knight Armor | ItemParam |
| `23300102` | Old Knight Gauntlets | ItemParam |
| `23300103` | Old Knight Leggings | ItemParam |
| `23310100` | Drakekeeper Helm | ItemParam |
| `23310101` | Drakekeeper Armor | ItemParam |
| `23310102` | Drakekeeper Gauntlets | ItemParam |
| `23310103` | Drakekeeper Boots | ItemParam |
| `23320100` | Throne Defender Helm | ItemParam |
| `23320101` | Throne Defender Armor | ItemParam |
| `23320102` | Throne Defender Gauntlets | ItemParam |
| `23320103` | Throne Defender Leggings | ItemParam |
| `23330100` | Velstadt's Helm | ItemParam |
| `23330101` | Velstadt's Armor | ItemParam |
| `23330102` | Velstadt's Gauntlets | ItemParam |
| `23330103` | Velstadt's Leggings | ItemParam |
| `23340100` | Throne Watcher Helm | ItemParam |
| `23340101` | Throne Watcher Armor | ItemParam |
| `23340102` | Throne Watcher Gauntlets | ItemParam |
| `23340103` | Throne Watcher Leggings | ItemParam |
| `25040100` | Looking Glass Mask | ItemParam |
| `25040101` | Looking Glass Armor | ItemParam |
| `25040102` | Looking Glass Gauntlets | ItemParam |
| `25040103` | Looking Glass Leggings | ItemParam |
| `25060101` | Agdayne's Black Robe | ItemParam |
| `25060102` | Agdayne's Cuffs | ItemParam |
| `25060103` | Agdayne's Kilt | ItemParam |
| `25090100` | Leydia Black Hood | ItemParam |
| `25090101` | Leydia Black Robe | ItemParam |
| `25100100` | Insolent Helm | ItemParam |
| `25100101` | Insolent Armor | ItemParam |
| `25100102` | Insolent Gloves | ItemParam |
| `25100103` | Insolent Boots | ItemParam |
| `25110100` | Imperious Helm | ItemParam |
| `25110101` | Imperious Armor | ItemParam |
| `25110102` | Imperious Gloves | ItemParam |
| `25110103` | Imperious Leggings | ItemParam |
| `25120100` | Leydia White Hood | ItemParam |
| `25120101` | Leydia White Robe | ItemParam |
| `25120102` | Leydia Gauntlets | ItemParam |
| `25130100` | King's Crown | ItemParam |
| `25130101` | King's Armor | ItemParam |
| `25130102` | King's Gauntlets | ItemParam |
| `25130103` | King's Leggings | ItemParam |
| `26100100` | Dragonrider Helm | ItemParam |
| `26100101` | Dragonrider Armor | ItemParam |
| `26100102` | Dragonrider Gauntlets | ItemParam |
| `26100103` | Dragonrider Leggings | ItemParam |
| `26180100` | Executioner Helm | ItemParam |
| `26180101` | Executioner Armor | ItemParam |
| `26180102` | Executioner Gauntlets | ItemParam |
| `26180103` | Executioner Leggings | ItemParam |
| `26260100` | Penal Mask | ItemParam |
| `26260101` | Penal Straightjacket | ItemParam |
| `26260102` | Penal Handcuffs | ItemParam |
| `26260103` | Penal Skirt | ItemParam |
| `26510100` | Fume Sorcerer Mask | ItemParam |
| `26510101` | Fume Sorcerer Robes | ItemParam |
| `26510102` | Fume Sorcerer Gloves | ItemParam |
| `26510103` | Fume Sorcerer Boots | ItemParam |
| `26590100` | Rampart Golem Helm | ItemParam |
| `26590101` | Rampart Golem Armor | ItemParam |
| `26590102` | Rampart Golem Gauntlets | ItemParam |
| `26590103` | Rampart Golem Leggings | ItemParam |
| `26650100` | Sanctum Knight Helm | ItemParam |
| `26650101` | Sanctum Knight Armor | ItemParam |
| `26650102` | Sanctum Knight Gauntlets | ItemParam |
| `26650103` | Sanctum Knight Leggings | ItemParam |
| `26660102` | Sanctum Soldier Gauntlet | ItemParam |
| `26700100` | Sanctum Priestess Tiara | ItemParam |
| `26750100` | Raime's Helm | ItemParam |
| `26750101` | Raime's Armor | ItemParam |
| `26750102` | Raime's Gauntlets | ItemParam |
| `26750103` | Raime's Leggings | ItemParam |
| `26770101` | Retainer Robe | ItemParam |
| `26800100` | Alonne's Helm | ItemParam |
| `26800101` | Alonne's Armor | ItemParam |
| `26800102` | Alonne's Gauntlets | ItemParam |
| `26800103` | Alonne's Leggings | ItemParam |
| `26880100` | Loyce Helm | ItemParam |
| `26880101` | Loyce Armor | ItemParam |
| `26880102` | Loyce Gauntlets | ItemParam |
| `26880103` | Loyce Leggings | ItemParam |
| `26890100` | Charred Loyce Helm | ItemParam |
| `26890101` | Charred Loyce Armor | ItemParam |
| `26890102` | Charred Loyce Gauntlets | ItemParam |
| `26890103` | Charred Loyce Leggings | ItemParam |
| `26900100` | Ivory King Helm | ItemParam |
| `26900101` | Ivory King Armor | ItemParam |
| `26900102` | Ivory King Gauntlets | ItemParam |
| `26900103` | Ivory King Leggings | ItemParam |
| `27210101` | Llewellyn Armor | ItemParam |
| `27210102` | Llewellyn Gloves | ItemParam |
| `27210103` | Llewellyn Shoes | ItemParam |
| `27240100` | Drangleic Helm | ItemParam |
| `27240101` | Drangleic Mail | ItemParam |
| `27240102` | Drangleic Gauntlets | ItemParam |
| `27240103` | Drangleic Leggings | ItemParam |
| `27420100` | Creighton's Steel Mask | ItemParam |
| `27420101` | Creighton's Chainmail | ItemParam |
| `27420102` | Creighton's Chain Gloves | ItemParam |
| `27420103` | Creighton's Chain Leggings | ItemParam |
| `27430100` | Benhart's Knight Helm | ItemParam |
| `27430101` | Benhart's Armor | ItemParam |
| `27430102` | Benhart's Gauntlets | ItemParam |
| `27430103` | Benhart's Boots | ItemParam |
| `27440100` | Standard Helm | ItemParam |
| `27440101` | Hard Leather Armor | ItemParam |
| `27440102` | Hard Leather Gauntlets | ItemParam |
| `27440103` | Hard Leather Boots | ItemParam |
| `27510100` | Cale's Helm | ItemParam |
| `27510101` | Cale's Leather Armor | ItemParam |
| `27510103` | Cale's Shoes | ItemParam |
| `27520100` | Lucatiel's Mask | ItemParam |
| `27520101` | Lucatiel's Vest | ItemParam |
| `27520102` | Lucatiel's Gloves | ItemParam |
| `27520103` | Lucatiel's Trousers | ItemParam |
| `27521100` | Mirrah Hat | ItemParam |
| `27530100` | Bell Keeper Helmet | ItemParam |
| `27530101` | Bell Keeper Bellyband | ItemParam |
| `27530102` | Bell Keeper Cuffs | ItemParam |
| `27530103` | Bell Keeper Trousers | ItemParam |
| `27550100` | Mad Warrior Mask | ItemParam |
| `27550101` | Mad Warrior Armor | ItemParam |
| `27550102` | Mad Warrior Gauntlets | ItemParam |
| `27550103` | Mad Warrior Leggings | ItemParam |
| `27680100` | Black Hood | ItemParam |
| `27680101` | Black Robes | ItemParam |
| `27680102` | Black Gloves | ItemParam |
| `27680103` | Black Boots | ItemParam |
| `27690100` | Saint's Hood | ItemParam |
| `27690101` | Saint's Dress | ItemParam |
| `27690102` | Saint's Long Gloves | ItemParam |
| `27690103` | Saint's Trousers | ItemParam |
| `27700100` | Hexer's Hood | ItemParam |
| `27700101` | Hexer's Robes | ItemParam |
| `27700102` | Hexer's Gloves | ItemParam |
| `27700103` | Hexer's Boots | ItemParam |
| `27710100` | Chaos Hood | ItemParam |
| `27710101` | Chaos Robe | ItemParam |
| `27710102` | Chaos Gloves | ItemParam |
| `27710103` | Chaos Boots | ItemParam |
| `27830100` | Nahr Alma Hood | ItemParam |
| `27830101` | Nahr Alma Robes | ItemParam |
| `27950100` | Targray's Helm | ItemParam |
| `27950101` | Targray's Armor | ItemParam |
| `27950102` | Targray's Manifers | ItemParam |
| `27950103` | Targray's Leggings | ItemParam |

### Spells

| id | name | defined in |
|---|---|---|
| `31010000` | Soul Arrow | ItemParam |
| `31020000` | Great Soul Arrow | ItemParam |
| `31030000` | Heavy Soul Arrow | ItemParam |
| `31040000` | Great Heavy Soul Arrow | ItemParam |
| `31050000` | Homing Soul Arrow | ItemParam |
| `31060000` | Heavy Homing Soul Arrow | ItemParam |
| `31070000` | Homing Soulmass | ItemParam |
| `31080000` | Homing Crystal Soulmass | ItemParam |
| `31090000` | Soul Spear | ItemParam |
| `31100000` | Crystal Soul Spear | ItemParam |
| `31110000` | Shockwave | ItemParam |
| `31120000` | Soul Spear Barrage | ItemParam |
| `31130000` | Soul Shower | ItemParam |
| `31140000` | Soul Greatsword | ItemParam |
| `31150000` | Soul Vortex | ItemParam |
| `31160000` | Soul Bolt | ItemParam |
| `31170000` | Soul Geyser | ItemParam |
| `31180000` | Magic Weapon | ItemParam |
| `31190000` | Great Magic Weapon | ItemParam |
| `31200000` | Crystal Magic Weapon | ItemParam |
| `31210000` | Strong Magic Shield | ItemParam |
| `31220000` | Yearn | ItemParam |
| `31230000` | Hush | ItemParam |
| `31240000` | Fall Control | ItemParam |
| `31250000` | Hidden Weapon | ItemParam |
| `31260000` | Repair | ItemParam |
| `31270000` | Cast Light | ItemParam |
| `31280000` | Chameleon | ItemParam |
| `31290000` | Unleash Magic | ItemParam |
| `31300000` | Soul Flash | ItemParam |
| `31310000` | Focus Souls | ItemParam |
| `32010000` | Heal | ItemParam |
| `32020000` | Med Heal | ItemParam |
| `32030000` | Great Heal Excerpt | ItemParam |
| `32040000` | Great Heal | ItemParam |
| `32050000` | Soothing Sunlight | ItemParam |
| `32060000` | Replenishment | ItemParam |
| `32070000` | Resplendent Life | ItemParam |
| `32080000` | Bountiful Sunlight | ItemParam |
| `32090000` | Caressing Prayer | ItemParam |
| `32100000` | Force | ItemParam |
| `32110000` | Wrath of the Gods | ItemParam |
| `32120000` | Emit Force | ItemParam |
| `32130000` | Heavenly Thunder | ItemParam |
| `32140000` | Lightning Spear | ItemParam |
| `32150000` | Great Lightning Spear | ItemParam |
| `32160000` | Sunlight Spear | ItemParam |
| `32170000` | Soul Appease | ItemParam |
| `32180000` | Blinding Bolt | ItemParam |
| `32190000` | Magic Barrier | ItemParam |
| `32200000` | Great Magic Barrier | ItemParam |
| `32210000` | Homeward | ItemParam |
| `32220000` | Guidance | ItemParam |
| `32230000` | Sacred Oath | ItemParam |
| `32240000` | Unveil | ItemParam |
| `32250000` | Perseverance | ItemParam |
| `32260000` | Sunlight Blade | ItemParam |
| `32300000` | Denial | ItemParam |
| `32310000` | Splintering Lightning Spear | ItemParam |
| `33010000` | Fireball | ItemParam |
| `33020000` | Fire Orb | ItemParam |
| `33030000` | Great Fireball | ItemParam |
| `33040000` | Great Chaos Fireball | ItemParam |
| `33050000` | Firestorm | ItemParam |
| `33060000` | Fire Tempest | ItemParam |
| `33070000` | Chaos Storm | ItemParam |
| `33080000` | Combustion | ItemParam |
| `33090000` | Great Combustion | ItemParam |
| `33100000` | Fire Whip | ItemParam |
| `33110000` | Poison Mist | ItemParam |
| `33120000` | Toxic Mist | ItemParam |
| `33130000` | Acid Surge | ItemParam |
| `33140000` | Lingering Flame | ItemParam |
| `33150000` | Flame Swathe | ItemParam |
| `33160000` | Forbidden Sun | ItemParam |
| `33170000` | Flame Weapon | ItemParam |
| `33180000` | Flash Sweat | ItemParam |
| `33190000` | Iron Flesh | ItemParam |
| `33200000` | Warmth | ItemParam |
| `33210000` | Immolation | ItemParam |
| `33300000` | Fire Snake | ItemParam |
| `33310000` | Dance of Fire | ItemParam |
| `33320000` | Outcry | ItemParam |
| `34010000` | Dark Orb | ItemParam |
| `34020000` | Dark Hail | ItemParam |
| `34030000` | Dark Fog | ItemParam |
| `34040000` | Affinity | ItemParam |
| `34050000` | Dead Again | ItemParam |
| `34060000` | Dark Weapon | ItemParam |
| `34070000` | Whisper of Despair | ItemParam |
| `34080000` | Repel | ItemParam |
| `34090000` | Twisted Barricade | ItemParam |
| `34100000` | Numbness | ItemParam |
| `34300000` | Dark Greatsword | ItemParam |
| `34310000` | Recollection | ItemParam |
| `35010000` | Scraps of Life | ItemParam |
| `35020000` | Darkstorm | ItemParam |
| `35030000` | Resonant Soul | ItemParam |
| `35040000` | Great Resonant Soul | ItemParam |
| `35050000` | Climax | ItemParam |
| `35060000` | Resonant Flesh | ItemParam |
| `35070000` | Resonant Weapon | ItemParam |
| `35080000` | Lifedrain Patch | ItemParam |
| `35090000` | Profound Still | ItemParam |
| `35300000` | Promised Walk of Peace | ItemParam |
| `35310000` | Dark Dance | ItemParam |

### Rings

| id | name | defined in |
|---|---|---|
| `40010000` | Life Ring | ItemParam, RingParam |
| `40010001` | Life Ring+1 | ItemParam, RingParam |
| `40010002` | Life Ring+2 | ItemParam, RingParam |
| `40010003` | Life Ring+3 | ItemParam, RingParam |
| `40020000` | Chloranthy Ring | ItemParam, RingParam |
| `40020001` | Chloranthy Ring+1 | ItemParam, RingParam |
| `40020002` | Chloranthy Ring+2 | ItemParam, RingParam |
| `40030000` | Royal Soldier's Ring | ItemParam, RingParam |
| `40030001` | Royal Soldier's Ring+1 | ItemParam, RingParam |
| `40030002` | Royal Soldier's Ring+2 | ItemParam, RingParam |
| `40040000` | First Dragon Ring | ItemParam, RingParam |
| `40040001` | Second Dragon Ring | ItemParam, RingParam |
| `40040002` | Third Dragon Ring | ItemParam, RingParam |
| `40050000` | Ring of Steel Protection | ItemParam, RingParam |
| `40050001` | Ring of Steel Protection+1 | ItemParam, RingParam |
| `40050002` | Ring of Steel Protection+2 | ItemParam, RingParam |
| `40060000` | Spell Quartz Ring | ItemParam, RingParam |
| `40060001` | Spell Quartz Ring+1 | ItemParam, RingParam |
| `40060002` | Spell Quartz Ring+2 | ItemParam, RingParam |
| `40060003` | Spell Quartz Ring+3 | ItemParam, RingParam |
| `40070000` | Flame Quartz Ring | ItemParam, RingParam |
| `40070001` | Flame Quartz Ring+1 | ItemParam, RingParam |
| `40070002` | Flame Quartz Ring+2 | ItemParam, RingParam |
| `40070003` | Flame Quartz Ring+3 | ItemParam, RingParam |
| `40080000` | Thunder Quartz Ring | ItemParam, RingParam |
| `40080001` | Thunder Quartz Ring+1 | ItemParam, RingParam |
| `40080002` | Thunder Quartz Ring+2 | ItemParam, RingParam |
| `40080003` | Thunder Quartz Ring+3 | ItemParam, RingParam |
| `40090000` | Dark Quartz Ring | ItemParam, RingParam |
| `40090001` | Dark Quartz Ring+1 | ItemParam, RingParam |
| `40090002` | Dark Quartz Ring+2 | ItemParam, RingParam |
| `40090003` | Dark Quartz Ring+3 | ItemParam, RingParam |
| `40100000` | Poisonbite Ring | ItemParam, RingParam |
| `40100001` | Poisonbite Ring+1 | ItemParam, RingParam |
| `40110000` | Bloodbite Ring | ItemParam, RingParam |
| `40110001` | Bloodbite Ring+1 | ItemParam, RingParam |
| `40120000` | Bracing Knuckle Ring | ItemParam, RingParam |
| `40120001` | Bracing Knuckle Ring+1 | ItemParam, RingParam |
| `40120002` | Bracing Knuckle Ring+2 | ItemParam, RingParam |
| `40130000` | Cursebite Ring | ItemParam, RingParam |
| `40135000` | Ash Knuckle Ring | ItemParam, RingParam |
| `40140000` | Dispelling Ring | ItemParam, RingParam |
| `40140001` | Dispelling Ring+1 | ItemParam, RingParam |
| `40150000` | Ring of Resistance | ItemParam, RingParam |
| `40150001` | Ring of Resistance+1 | ItemParam, RingParam |
| `40160000` | Ring of Blades | ItemParam, RingParam |
| `40160001` | Ring of Blades+1 | ItemParam, RingParam |
| `40160002` | Ring of Blades+2 | ItemParam, RingParam |
| `40210000` | Ring of Knowledge | ItemParam, RingParam |
| `40220000` | Ring of Prayer | ItemParam, RingParam |
| `40230000` | Stone Ring | ItemParam, RingParam |
| `40260000` | Red Tearstone Ring | ItemParam, RingParam |
| `40280000` | Blue Tearstone Ring | ItemParam, RingParam |
| `40290000` | Ring of Giants | ItemParam, RingParam |
| `40290001` | Ring of Giants+1 | ItemParam, RingParam |
| `40290002` | Ring of Giants+2 | ItemParam, RingParam |
| `40295000` | Old Leo Ring | ItemParam, RingParam |
| `40300000` | Ring of Soul Protection | ItemParam, RingParam |
| `40310000` | Ring of Life Protection | ItemParam, RingParam |
| `40320000` | Lingering Dragoncrest Ring | ItemParam, RingParam |
| `40320001` | Lingering Dragoncrest Ring+1 | ItemParam, RingParam |
| `40320002` | Lingering Dragoncrest Ring+2 | ItemParam, RingParam |
| `40330000` | Clear Bluestone Ring | ItemParam, RingParam |
| `40330001` | Clear Bluestone Ring+1 | ItemParam, RingParam |
| `40330002` | Clear Bluestone Ring+2 | ItemParam, RingParam |
| `40340000` | Northern Ritual Band | ItemParam, RingParam |
| `40340001` | Northern Ritual Band+1 | ItemParam, RingParam |
| `40340002` | Northern Ritual Band+2 | ItemParam, RingParam |
| `40350000` | Southern Ritual Band | ItemParam, RingParam |
| `40350001` | Southern Ritual Band+1 | ItemParam, RingParam |
| `40350002` | Southern Ritual Band+2 | ItemParam, RingParam |
| `40360000` | Covetous Gold Serpent Ring | ItemParam, RingParam |
| `40360001` | Covetous Gold Serpent Ring+1 | ItemParam, RingParam |
| `40360002` | Covetous Gold Serpent Ring+2 | ItemParam, RingParam |
| `40370000` | Covetous Silver Serpent Ring | ItemParam, RingParam |
| `40370001` | Covetous Silver Serpent Ring+1 | ItemParam, RingParam |
| `40370002` | Covetous Silver Serpent Ring+2 | ItemParam, RingParam |
| `40390000` | Ring of the Evil Eye | ItemParam, RingParam |
| `40390001` | Ring of the Evil Eye+1 | ItemParam, RingParam |
| `40390002` | Ring of the Evil Eye+2 | ItemParam, RingParam |
| `40400000` | Ring of Restoration | ItemParam, RingParam |
| `40410000` | Ring of Binding | ItemParam, RingParam |
| `40420000` | Silvercat Ring | ItemParam, RingParam |
| `40430000` | Redeye Ring | ItemParam, RingParam |
| `40440000` | Gower's Ring of Protection | ItemParam, RingParam |
| `40450000` | Name-engraved Ring | ItemParam, RingParam |
| `40460000` | Slumbering Dragoncrest Ring | ItemParam, RingParam |
| `40470000` | Hawk Ring | ItemParam, RingParam |
| `40480000` | Old Sun Ring | ItemParam, RingParam |
| `40500000` | Illusory Ring of a Conqueror | ItemParam, RingParam |
| `40510000` | King's Ring | ItemParam, RingParam |
| `40520000` | Ring of the Dead | ItemParam, RingParam |
| `40530000` | Ring of Thorns | ItemParam, RingParam |
| `40530001` | Ring of Thorns+1 | ItemParam, RingParam |
| `40530002` | Ring of Thorns+2 | ItemParam, RingParam |
| `40540000` | Delicate String | ItemParam, RingParam |
| `40550000` | White Ring | ItemParam, RingParam |
| `40560000` | Illusory Ring of the Vengeful | RingParam |
| `40570000` | Illusory Ring of the Guilty | RingParam |
| `40610000` | Ring of Whispers | ItemParam, RingParam |
| `40620000` | Illusory Ring of the Exalted | ItemParam, RingParam |
| `40700000` | Crest of the Rat | ItemParam, RingParam |
| `40710000` | Bell Keeper's Seal | ItemParam, RingParam |
| `40720000` | Guardian's Seal | ItemParam, RingParam |
| `40730000` | Crest of Blood | ItemParam, RingParam |
| `40740000` | Blue Seal | ItemParam, RingParam |
| `40750000` | Abyss Seal | ItemParam, RingParam |
| `40760000` | Vanquisher's Seal | ItemParam, RingParam |
| `40770000` | Sun Seal | ItemParam, RingParam |
| `40780000` | Ancient Dragon Seal | ItemParam, RingParam |
| `41000000` | Simpleton's Ring | ItemParam, RingParam |
| `41010000` | Strength Ring | ItemParam, RingParam |
| `41020000` | Dexterity Ring | ItemParam, RingParam |
| `41030000` | Ivory Warrior Ring | ItemParam, RingParam |
| `41040000` | Sorcery Clutch Ring | ItemParam, RingParam |
| `41050000` | Lightning Clutch Ring | ItemParam, RingParam |
| `41060000` | Fire Clutch Ring | ItemParam, RingParam |
| `41070000` | Dark Clutch Ring | ItemParam, RingParam |
| `41090000` | Baneful Bird Ring | ItemParam, RingParam |
| `41100000` | Flynn's Ring | ItemParam, RingParam |
| `41110000` | Ring of the Embedded | ItemParam, RingParam |
| `41120000` | Ring of the Living | ItemParam, RingParam |
| `41130000` | Yorgh's Ring | ItemParam, RingParam |
| `42000000` | Agape Ring | ItemParam, RingParam |

### Keys and quest items

| id | name | defined in |
|---|---|---|
| `50600000` | Soldier Key | ItemParam |
| `50610000` | Key to King's Passage | ItemParam |
| `50800000` | Bastille Key | ItemParam |
| `50810000` | Iron Key | ItemParam |
| `50820000` | Forgotten Key | ItemParam |
| `50830000` | Brightstone Key | ItemParam |
| `50840000` | Antiquated Key | ItemParam |
| `50850000` | Fang Key | ItemParam |
| `50860000` | House Key | ItemParam |
| `50870000` | Lenigrast's Key | ItemParam |
| `50880000` | Smooth & Silky Stone | ItemParam |
| `50885000` | Small Smooth & Silky Stone | ItemParam |
| `50890000` | Rotunda Lockstone | ItemParam |
| `50900000` | Giant's Kinship | ItemParam |
| `50910000` | Ashen Mist Heart | ItemParam |
| `50920000` | Soul of a Giant | ItemParam |
| `50930000` | Tseldora Den Key | ItemParam |
| `50940000` | Champion's Tablet | ItemParam |
| `50950000` | Ladder Miniature | ItemParam |
| `50960000` | Soul Vessel | ItemParam |
| `50970000` | Undead Lockaway Key | ItemParam |
| `50990000` | Dull Ember | ItemParam |
| `51000000` | Crushed Eye Orb | ItemParam |
| `51010000` | Simpleton's Spice | ItemParam |
| `51020000` | Skeptic's Spice | ItemParam |
| `51030000` | Aldia Key | ItemParam |
| `52000000` | Dragon Talon | ItemParam |
| `52100000` | Heavy Iron Key | ItemParam |
| `52200000` | Frozen Flower | ItemParam |
| `52300000` | Eternal Sanctum Key | ItemParam |
| `52400000` | Tower Key | ItemParam |
| `52500000` | Garrison Ward Key | ItemParam |
| `52650000` | Dragon Stone | ItemParam |
| `53100000` | Scorching Iron Scepter | ItemParam |
| `53200000` | Smelter Wedge | ItemParam |
| `53300000` | Soul of Nadalia, Bride of Ash | ItemParam |
| `53600000` | Eye of the Priestess | ItemParam |

### Consumables

| id | name | defined in |
|---|---|---|
| `60010000` | Lifegem | ItemParam |
| `60020000` | Radiant Lifegem | ItemParam |
| `60030000` | Old Radiant Lifegem | ItemParam |
| `60035000` | Elizabeth Mushroom | ItemParam |
| `60036000` | Dried Root | ItemParam |
| `60040000` | Amber Herb | ItemParam |
| `60050000` | Twilight Herb | ItemParam |
| `60060000` | Wilted Dusk Herb | ItemParam |
| `60070000` | Poison Moss | ItemParam |
| `60090000` | Monastery Charm | ItemParam |
| `60100000` | Dragon Charm | ItemParam |
| `60105000` | Divine Blessing | ItemParam |
| `60110000` | Rouge Water | ItemParam |
| `60120000` | Crimson Water | ItemParam |
| `60151000` | Human Effigy | ItemParam |
| `60155000` | Estus Flask | ItemParam |
| `60160000` | Small Blue Burr | ItemParam |
| `60170000` | Small Yellow Burr | ItemParam |
| `60180000` | Small Orange Burr | ItemParam |
| `60190000` | Dark Troches | ItemParam |
| `60200000` | Common Fruit | ItemParam |
| `60210000` | Red Leech Troches | ItemParam |
| `60230000` | Triclops Snake Troches | ItemParam |
| `60235000` | Old Growth Balm | ItemParam |
| `60236000` | Vine Balm | ItemParam |
| `60237000` | Blackweed Balm | ItemParam |
| `60238000` | Goldenfruit Balm | ItemParam |
| `60239000` | Brightbug | ItemParam |
| `60240000` | Aromatic Ooze | ItemParam |
| `60250000` | Gold Pine Resin | ItemParam |
| `60260000` | Charcoal Pine Resin | ItemParam |
| `60270000` | Dark Pine Resin | ItemParam |
| `60280000` | Rotten Pine Resin | ItemParam |
| `60290000` | Bleeding Serum | ItemParam |
| `60310000` | Green Blossom | ItemParam |
| `60320000` | Rusted Coin | ItemParam |
| `60350000` | Homeward Bone | ItemParam |
| `60355000` | Aged Feather | ItemParam |
| `60360000` | Darksign | ItemParam |
| `60370000` | Silver Talisman | ItemParam |
| `60405000` | Dragon Head Stone | ItemParam |
| `60405010` | Dragon Head Stone | ItemParam |
| `60406000` | Dragon Torso Stone | ItemParam |
| `60406010` | Dragon Torso Stone | ItemParam |
| `60410000` | Repair Powder | ItemParam |
| `60420000` | Torch | ItemParam |
| `60430000` | Flame Butterfly | ItemParam |
| `60450000` | Prism Stone | ItemParam |
| `60470000` | Hello Carving | ItemParam |
| `60480000` | Thank You Carving | ItemParam |
| `60490000` | I'm Sorry Carving | ItemParam |
| `60500000` | Very Good! Carving | ItemParam |
| `60510000` | Rubbish | ItemParam |
| `60511000` | Petrified Something | ItemParam |
| `60525000` | Estus Flask Shard | ItemParam |
| `60526000` | Sublime Bone Dust | ItemParam |
| `60527000` | Bonfire Ascetic | ItemParam |
| `60530000` | Alluring Skull | ItemParam |
| `60531000` | Lloyd's Talisman | ItemParam |
| `60536000` | Pharros' Lockstone | ItemParam |
| `60537000` | Fragrant Branch of Yore | ItemParam |
| `60538000` | Fire Seed | ItemParam |
| `60540000` | Throwing Knife | ItemParam |
| `60550000` | Witching Urn | ItemParam |
| `60560000` | Lightning Urn | ItemParam |
| `60570000` | Firebomb | ItemParam |
| `60575000` | Black Firebomb | ItemParam |
| `60580000` | Hexing Urn | ItemParam |
| `60590000` | Poison Throwing Knife | ItemParam |
| `60595000` | Dung Pie | ItemParam |
| `60600000` | Lacerating Knife | ItemParam |
| `60610000` | Corrosive Urn | ItemParam |
| `60620000` | Holy Water Urn | ItemParam |
| `60625000` | Fading Soul | ItemParam |
| `60630000` | Soul of a Lost Undead | ItemParam |
| `60640000` | Large Soul of a Lost Undead | ItemParam |
| `60650000` | Soul of a Nameless Soldier | ItemParam |
| `60660000` | Large Soul of a Nameless Soldier | ItemParam |
| `60670000` | Soul of a Proud Knight | ItemParam |
| `60680000` | Large Soul of a Proud Knight | ItemParam |
| `60690000` | Soul of a Brave Warrior | ItemParam |
| `60700000` | Large Soul of a Brave Warrior | ItemParam |
| `60710000` | Soul of a Hero | ItemParam |
| `60720000` | Soul of a Great Hero | ItemParam |
| `60760000` | Wood Arrow | ItemParam |
| `60770000` | Iron Arrow | ItemParam |
| `60780000` | Magic Arrow | ItemParam |
| `60790000` | Lightning Arrow | ItemParam |
| `60800000` | Fire Arrow | ItemParam |
| `60810000` | Dark Arrow | ItemParam |
| `60820000` | Poison Arrow | ItemParam |
| `60830000` | Lacerating Arrow | ItemParam |
| `60850000` | Iron Greatarrow | ItemParam |
| `60870000` | Lightning Greatarrow | ItemParam |
| `60880000` | Fire Greatarrow | ItemParam |
| `60900000` | Destructive Greatarrow | ItemParam |
| `60910000` | Wood Bolt | ItemParam |
| `60920000` | Heavy Bolt | ItemParam |
| `60930000` | Magic Bolt | ItemParam |
| `60940000` | Lightning Bolt | ItemParam |
| `60950000` | Fire Bolt | ItemParam |
| `60960000` | Dark Bolt | ItemParam |
| `60970000` | Titanite Shard | ItemParam |
| `60975000` | Large Titanite Shard | ItemParam |
| `60980000` | Titanite Chunk | ItemParam |
| `60990000` | Titanite Slab | ItemParam |
| `61000000` | Twinkling Titanite | ItemParam |
| `61030000` | Petrified Dragon Bone | ItemParam |
| `61060000` | Faintstone | ItemParam |
| `61070000` | Boltstone | ItemParam |
| `61080000` | Firedrake Stone | ItemParam |
| `61090000` | Darknight Stone | ItemParam |
| `61100000` | Poison Stone | ItemParam |
| `61110000` | Bleed Stone | ItemParam |
| `61130000` | Raw Stone | ItemParam |
| `61140000` | Magic Stone | ItemParam |
| `61150000` | Old Mundane Stone | ItemParam |
| `61160000` | Palestone | ItemParam |

### Multiplayer items

| id | name | defined in |
|---|---|---|
| `62000000` | Dried Fingers | ItemParam |
| `62020000` | Bone of Order | ItemParam |
| `62030000` | White Sign Soapstone | ItemParam |
| `62040000` | Small White Sign Soapstone | ItemParam |
| `62045000` | Red Sign Soapstone | ItemParam |
| `62050000` | Cracked Blue Eye Orb | ItemParam |
| `62060000` | Cracked Red Eye Orb | ItemParam |
| `62070000` | Dragon Eye | ItemParam |
| `62100000` | Token of Fidelity | ItemParam |
| `62110000` | Token of Spite | ItemParam |
| `62120000` | Sunlight Medal | ItemParam |
| `62130000` | Dragon Scale | ItemParam |
| `62140000` | Rat Tail | ItemParam |
| `62150000` | Awestone | ItemParam |
| `62160000` | Black Separation Crystal | ItemParam |
| `62170000` | Seed of a Tree of Giants | ItemParam |
| `62190000` | Petrified Egg | ItemParam |

### Gestures

| id | name | defined in |
|---|---|---|
| `63000000` | Point Gesture | ItemParam |
| `63001000` | I won't bite Gesture | ItemParam |
| `63003000` | Bow Gesture | ItemParam |
| `63004000` | Welcome Gesture | ItemParam |
| `63005000` | Duel bow Gesture | ItemParam |
| `63006000` | Wave Gesture | ItemParam |
| `63007000` | Pumped up Gesture | ItemParam |
| `63008000` | Joy Gesture | ItemParam |
| `63009000` | Warcry Gesture | ItemParam |
| `63010000` | Warmup Gesture | ItemParam |
| `63011000` | Hurrah! Gesture | ItemParam |
| `63012000` | Righty-ho! Gesture | ItemParam |
| `63013000` | No way Gesture | ItemParam |
| `63014000` | This one's me Gesture | ItemParam |
| `63015000` | Have mercy! Gesture | ItemParam |
| `63016000` | Prostration Gesture | ItemParam |
| `63017000` | Decapitate Gesture | ItemParam |
| `63018000` | Fist pump Gesture | ItemParam |
| `63019000` | Mock Gesture | ItemParam |
| `63021000` | Praise the Sun Gesture | ItemParam |

### Boss souls

| id | name | defined in |
|---|---|---|
| `64000000` | Soul of the Pursuer | ItemParam |
| `64010000` | Soul of the Last Giant | ItemParam |
| `64020000` | Dragonrider Soul | ItemParam |
| `64030000` | Old Dragonslayer Soul | ItemParam |
| `64040000` | Flexile Sentry Soul | ItemParam |
| `64050000` | Ruin Sentinel Soul | ItemParam |
| `64060000` | Soul of the Lost Sinner | ItemParam |
| `64070000` | Executioner's Chariot Soul | ItemParam |
| `64080000` | Skeleton Lord's Soul | ItemParam |
| `64090000` | Covetous Demon Soul | ItemParam |
| `64100000` | Mytha, the Baneful Queen Soul | ItemParam |
| `64110000` | Smelter Demon Soul | ItemParam |
| `64120000` | Old Iron King Soul | ItemParam |
| `64130000` | Royal Rat Vanguard Soul | ItemParam |
| `64140000` | Soul of the Rotten | ItemParam |
| `64150000` | Scorpioness Najka Soul | ItemParam |
| `64160000` | Royal Rat Authority Soul | ItemParam |
| `64170000` | Soul of the Duke's Dear Freja | ItemParam |
| `64180000` | Looking Glass Knight Soul | ItemParam |
| `64190000` | Demon of Song Soul | ItemParam |
| `64200000` | Soul of Velstadt | ItemParam |
| `64210000` | Soul of the King | ItemParam |
| `64220000` | Guardian Dragon Soul | ItemParam |
| `64230000` | Ancient Dragon Soul | ItemParam |
| `64240000` | Giant Lord Soul | ItemParam |
| `64250000` | Soul of Nashandra | ItemParam |
| `64260000` | Throne Defender Soul | ItemParam |
| `64270000` | Throne Watcher Soul | ItemParam |
| `64280000` | Darklurker Soul | ItemParam |
| `64290000` | Belfry Gargoyle Soul | ItemParam |
| `64300000` | Old Witch Soul | ItemParam |
| `64310000` | Old King Soul | ItemParam |
| `64320000` | Old Dead One Soul | ItemParam |
| `64330000` | Old Paledrake Soul | ItemParam |
| `64500000` | Soul of Sinh, the Slumbering Dragon | ItemParam |
| `64510000` | Soul of the Fume Knight | ItemParam |
| `64520000` | Soul of Aava, the King's Pet | ItemParam |
| `64530000` | Soul of Elana, Squalid Queen | ItemParam |
| `64540000` | Soul of Nadalia, Bride of Ash | ItemParam |
| `64550000` | Soul of Alsanna, Silent Oracle | ItemParam |
| `64560000` | Soul of Sir Alonne | ItemParam |
| `64580000` | Soul of the Ivory King | ItemParam |
| `64590000` | Soul of Zallen, the King's Pet | ItemParam |
| `64600000` | Loyce Soul | ItemParam |
| `64610000` | Soul of Lud, the King's Pet | ItemParam |

## Named but not found in this regulation's item params

These carry names in the game's database but no row in the four params read here.
The `892xxxxxx` and `900xxxxxx` blocks are the bulk of them and look like preset or
catalogue entries rather than real items. Untested — not known to be unusable.

| id | name |
|---|---|
| `1340000` | no text |
| `3510000` | Shadow Claws |
| `21041100` | Hunter's Hat |
| `21450101` | Barrel |
| `23041101` | Singer's Dress |
| `23042101` | Singer's Dress |
| `27630101` | Rosabeth's Dress |
| `40050003` | no text |
| `50640000` | Weapon Smithbox |
| `50650000` | Armor Smithbox |
| `52600000` | no text |
| `52700000` | no text |
| `52800000` | no text |
| `52900000` | no text |
| `53000000` | no text |
| `53400000` | no text |
| `53500000` | no text |
| `60156000` | Estus Flask |
| `60220000` | Yellow Sea Troches |
| `60245000` | Pungent Ooze |
| `60330000` | Rhoy's Stone |
| `60340000` | Rhoy's Stone of Knowledge |
| `60380000` | Gold Talisman |
| `60390000` | Fake Dead Talisman |
| `60400000` | Illusory Talisman |
| `60840000` | Wooden Greatarrow |
| `60860000` | Magic Greatarrow |
| `60890000` | Dark Greatarrow |
| `63002000` | Proper bow Gesture |
| `892001000` | Faraam Helm |
| `892001001` | Faraam Armor |
| `892001002` | Faraam Gauntlets |
| `892001003` | Faraam Boots |
| `892001100` | Chaos Hood |
| `892001101` | Chaos Robes |
| `892001102` | Chaos Gloves |
| `892001103` | Chaos Boots |
| `892001200` | Dragonknight Helm |
| `892001201` | Dragonknight Robes |
| `892001202` | Dragonknight Gloves |
| `892001203` | Dragonknight Boots |
| `892001300` | Black Hood |
| `892001301` | Black Robes |
| `892001302` | Black Gloves |
| `892001303` | Black Boots |
| `892001400` | Hunter's Hat |
| `892001401` | Leather Armor |
| `892001402` | Leather Gloves |
| `892001403` | Leather Boots |
| `892001500` | Knight Helm |
| `892001501` | Knight Armor |
| `892001502` | Knight Gauntlets |
| `892001503` | Knight Leggings |
| `892001600` | Rogue Hood |
| `892001601` | Rogue Armor |
| `892001602` | Rogue Gauntlets |
| `892001603` | Rogue Leggings |
| `892001700` | Prisoner's Hood |
| `892001701` | Prisoner's Tatters |
| `892001702` | Prisoner's Gloves |
| `892001703` | Prisoner's Waistcloth |
| `892001800` | Prisoner's Hood |
| `892001801` | Prisoner's Tatters |
| `892001802` | Prisoner's Gloves |
| `892001803` | Prisoner's Waistcloth |
| `892001900` | Hollow Mage Hood |
| `892001901` | Hollow Mage Robes |
| `892002000` | Defender Helm |
| `892002001` | Defender Mail |
| `892002002` | Defender's Gauntlet |
| `892002003` | Defender's Leggings |
| `892002101` | Llewellyn Armor |
| `892002102` | Llewellyn Gloves |
| `892002103` | Llewellyn Shoes |
| `892002200` | Despatcher's Hood |
| `892002201` | Despatcher's Robes |
| `900008400` | Dagger |
| `900008401` | Royal Dirk |
| `900008402` | Longsword |
| `900008403` | Shortsword |
| `900008404` | Dragonslayer's Crescent Axe |
| `900008405` | Greataxe |
| `900008406` | Dragonrider's Halberd |
| `900008407` | Witchtree Branch |
| `900008408` | Lizard Staff |
| `900008409` | Dragonknight's Bell |
| `900008410` | Bell of the Idol |
| `900008411` | Short Bow |
| `900008412` | Light Crossbow |
| `900008413` | Target Shield |
| `900008414` | Small Leather Shield |
| `900008415` | Golden Wing Shield |
| `900008416` | Dragonknight's Shield |
| `900008417` | Silver Eagle Kite Shield |
| `900008418` | Imperial Shield |
| `900008419` | Disc Bell |
| `900008420` | Pursuer's Greatsword |
| `900008421` | Zweihander |
| `900008422` | Winged Spear |
| `900008423` | Estoc |
| `900008424` | Bastard Sword |
| `900008425` | Knight's Greatsword |
| `900008426` | Inquisitor's Blade |

## Ids with no English name

657 rows across those params have no entry in
the name table. They are overwhelmingly `ArmorParam`'s `19xxxxxx` block and enemy-only
weapons — internal variants rather than anything a player is meant to hold. Listed by range
only, since a nameless id is not much use as a prize.

| range | count | example |
|---|---|---|
| `0000000`–`0999999` | 116 | `0` |
| `3000000`–`3999999` | 9 | `3400000` |
| `6000000`–`6999999` | 27 | `6000000` |
| `7000000`–`7999999` | 1 | `7010000` |
| `11000000`–`11999999` | 132 | `11001100` |
| `12000000`–`12999999` | 110 | `12000000` |
| `13000000`–`13999999` | 77 | `13010100` |
| `15000000`–`15999999` | 24 | `15040100` |
| `16000000`–`16999999` | 47 | `16100100` |
| `17000000`–`17999999` | 61 | `17210101` |
| `19000000`–`19999999` | 16 | `19000100` |
| `21000000`–`21999999` | 16 | `21001100` |
| `26000000`–`26999999` | 8 | `26510000` |
| `27000000`–`27999999` | 1 | `27521000` |
| `60000000`–`60999999` | 4 | `60155010` |
| `65000000`–`65999999` | 6 | `65240000` |
| `900000000`–`900999999` | 2 | `900008182` |
