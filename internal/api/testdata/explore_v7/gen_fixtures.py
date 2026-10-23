"""Generates internal/api/testdata/explore_v7/*.json (deterministic)."""
import json, random, sys, os

out = sys.argv[1]
os.makedirs(out, exist_ok=True)


def mbid(p, n):
    return f"{p}{n:07x}-0000-4000-8000-{n:012x}"


def dump(name, obj):
    with open(os.path.join(out, name), "w", encoding="utf-8", newline="\n") as f:
        json.dump(obj, f, indent=None if "large" in name else 1, sort_keys=True, separators=(",", ":") if "large" in name else None)
        f.write("\n")


# ---- household_indie: 25 artists, 3 obvious taste groups, A pinned ----
groups = [("shoegaze", "dream pop", 9), ("black metal", "doom metal", 8), ("bebop", "cool jazz", 8)]
artists = []
n = 0
for gi, (g1, g2, k) in enumerate(groups):
    for j in range(k):
        n += 1
        artists.append({"name": f"Indie {gi}-{j}", "mbid": mbid("a", 100 + n), "a": round(0.95 - 0.03 * j - 0.01 * gi, 4),
                        "genres": {g1: 6 + j % 3, g2: 4}, "group": gi})
for a in artists:
    a["similar"] = [{"mbid": b["mbid"], "name": b["name"], "s": round(0.9 - 0.02 * abs(a["a"] - b["a"]) * 10, 4)}
                    for b in artists if b["group"] == a["group"] and b is not a]
dump("household_indie.json", {"note": "25 artists in 3 taste groups; A and S pinned (mix clustering)",
                              "expect": ["Shoegaze & Dream Pop mix", "Black Metal & Doom Metal mix", "Bebop & Cool Jazz mix"],
                              "artists": artists})

# ---- household_tiny: 4 artists, no mix ----
tiny = [{"name": f"Tiny {i}", "mbid": mbid("a", 200 + i), "a": 0.5, "genres": {["rock", "jazz", "techno", "country"][i]: 3}, "similar": []} for i in range(4)]
dump("household_tiny.json", {"note": "4 artists in 4 genres: no mix", "artists": tiny})

# ---- API households ----
rng = random.Random(7)
GEN = [("rock", "alternative"), ("pop", "dance"), ("hip hop", "rap"), ("country", "folk"),
       ("alternative", "rock"), ("soul", "r&b"), ("latin", "pop"), ("dance", "house")]


def household(per_group, groups_n, follows, two_members=False, empty=False, seed=7):
    rng = random.Random(seed)
    libs, others, fol = [], [], {"luis": [], "ana": [], "bo": []}
    if empty:
        return {"follows": fol, "artists": [], "others": []}
    on = 0
    an = 0
    for g in range(groups_n):
        main, sub = GEN[g % len(GEN)]
        grp_others = []
        for j in range(70):
            on += 1
            o = {"name": f"Other {g}-{j}", "mbid": mbid("b", on), "genres": {main: 5 + j % 4, sub: 2},
                 "listeners": 2000 + 37 * j, "albums": []}
            for k in range(3 if j >= 60 else 2):
                an += 1
                o["albums"].append({"mbid": mbid("d", an), "title": f"Other {g}-{j} LP{k}", "type": "album",
                                    "date": f"{2010 + (j + k) % 14}-05-0{1 + k}", "listeners": 9000 - 100 * j - k})
            grp_others.append(o)
        others += grp_others
        grp = []
        for j in range(per_group):
            idx = g * per_group + j
            a = {"name": f"Artist {g}-{j}", "mbid": mbid("a", 1000 + idx), "genres": {main: 8 + j % 3, sub: 3}, "albums": [],
                 "listeners": 50000 - 90 * idx, "links": [], "guest": []}
            for k in range(4):
                an += 1
                a["albums"].append({"name": f"Artist {g}-{j} Record {k}", "mbid": mbid("c", an), "type": "album",
                                    "date": f"{2004 + (j + 3 * k) % 18}-0{1 + k}-1{k}", "have": k < (1 if two_members else 2),
                                    "listeners": 40000 - 9000 * k - 10 * j})
            if j % 5 == 0 or two_members and j % 2 == 0:  # a new release in the last weeks
                an += 1
                a["albums"].append({"name": f"Artist {g}-{j} New", "mbid": mbid("c", an), "type": "album", "days": -3 - j % 20, "listeners": 500})
            if j % 7 == 1:  # coming soon
                an += 1
                a["albums"].append({"name": f"Artist {g}-{j} Soon", "mbid": mbid("c", an), "type": "album", "days": 20 + j % 30, "listeners": 0})
            if j % 6 == 2:  # a round anniversary near 2026-10-23
                an += 1
                d = ["2001-10-30", "2006-11-05", "2016-10-20", "1996-11-10"][(j // 6) % 4]
                a["albums"].append({"name": f"Artist {g}-{j} Classic", "mbid": mbid("c", an), "type": "album", "date": d, "listeners": 30000 - j})
            if j < 3:  # members and collaborations (Together with)
                for k in range(3):
                    o = grp_others[60 + (j * 3 + k) % 10]
                    a["links"].append({"mbid": o["mbid"], "name": o["name"], "type": ["member of band", "founder", "collaboration"][k]})
            if j % 9 == 4:  # a guest spot
                an += 1
                host = grp_others[(j + 11) % 60]
                a["guest"].append({"mbid": mbid("d", 50000 + an), "title": f"{host['name']} feat. {a['name']}", "type": "single",
                                   "date": "2019-03-03", "listeners": 300, "artists": [host["mbid"], a["mbid"]], "artist_names": [host["name"], a["name"]]})
            grp.append(a)
        for j, a in enumerate(grp):
            sims = [b for b in grp if b is not a][:10]
            a["similar"] = [{"mbid": b["mbid"], "name": b["name"], "s": round(0.55 + 0.03 * (i % 10), 3)} for i, b in enumerate(sims)]
            a["similar"] += [{"mbid": o["mbid"], "name": o["name"], "s": round(0.95 - 0.02 * i, 3)} for i, o in enumerate(grp_others[(j * 7 + x) % 60] for x in range(20))]
        libs += grp
        if two_members:
            fol["luis" if g == 0 else "ana"] += [a["name"] for a in grp[:follows]] if g < 2 else []
        else:
            fol["luis"] += [a["name"] for a in grp[:follows]]
    return {"follows": fol, "artists": libs, "others": others}


large = household(50, 8, 5)
large["note"] = "400 artists in 8 genre groups; data for every row family (mixes, because, deep cuts, anniversaries, together, guest spots, finish the set, missing, new, coming soon, shelves, rising)"
dump("household_large.json", large)

two = household(12, 2, 8, two_members=True)
for i, a in enumerate(two["artists"]):
    a["genres"] = {"heavy metal": 9, "thrash metal": 3} if i < 12 else {"jazz": 9, "bebop": 3}
two["note"] = "member luis follows 8 metal artists, ana 8 jazz artists, bo nothing; one album had per artist"
dump("household_two_members.json", two)

empty = household(0, 0, 0, empty=True)
empty["note"] = "empty library: cold start"
dump("household_empty.json", empty)

ten = household(10, 1, 10, seed=3)
ten["note"] = "10 artists, all followed: Because rows drop rather than reuse a seed"
dump("household_ten.json", ten)
print("ok")
