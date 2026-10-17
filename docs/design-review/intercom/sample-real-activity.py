"""Apply real display metadata to existing disposable activity fixtures only."""
import json
import sqlite3
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
DB = Path(tempfile.gettempdir()) / 'holyarrs-intercom-preview-18747/config/holyarrs.db'
catalog = json.loads((HERE / 'sample-real-activity.json').read_text(encoding='utf-8'))
assert DB.is_file(), 'Disposable preview database is missing'
c = sqlite3.connect(DB, timeout=5)
c.execute('BEGIN IMMEDIATE')
try:
    assert c.execute('SELECT url FROM sources WHERE id=1').fetchone() == ('http://127.0.0.1:28767',)
    # Freeze all lifecycle fields; only descriptive fixture metadata may change.
    before = c.execute('SELECT * FROM downloads ORDER BY id').fetchall()
    slots = c.execute('SELECT * FROM slots ORDER BY id').fetchall()
    imports = c.execute('SELECT * FROM imports ORDER BY id').fetchall()
    replacements = {}
    for entry in catalog['shows'] + catalog['artists']:
        tid, name = entry['id'], entry['name']
        current = c.execute('SELECT name,kind FROM titles WHERE id=?', (tid,)).fetchone()
        assert current and current[0] in (entry['old_name'], name), f'Unexpected fixture title {tid}'
        replacements[entry['old_name']] = name
        replacements[entry['old_name'].replace(' ', '.')] = name.replace(' ', '.')
        year = entry.get('year', int(entry.get('albums', [{}])[0].get('release_date', '2000')[:4]))
        c.execute('UPDATE titles SET name=?,sort_name=?,original_name=?,year=?,artwork=?,aliases=? WHERE id=?',
                  (name, name.lower(), name, year, json.dumps([entry['artwork']]), '[]', tid))
        if current[1] == 'show':
            episodes = {(e['season'], e['number']): e for e in entry['episodes']}
            for iid, numbers, old_name in c.execute('SELECT id,numbers,name FROM items WHERE title_id=?', (tid,)).fetchall():
                n = json.loads(numbers)
                ep = episodes.get((n.get('season'), n.get('episode')))
                if ep:
                    c.execute('UPDATE items SET name=?,air_day=?,air_date=?,runtime=? WHERE id=?',
                              (ep['name'], ep['date'], ep['date']+'T00:00:00Z', ep['runtime'], iid))
                else:
                    # Unused fictional special has no real episode counterpart.
                    assert not c.execute("SELECT 1 FROM slots WHERE item_id=? AND state NOT IN ('wanted','missing')", (iid,)).fetchone()
                    c.execute("UPDATE items SET dropped_at=COALESCE(dropped_at,'2026-10-17T00:00:00Z') WHERE id=?", (iid,))
        else:
            part_ids = {6: [7, 8], 7: [11], 155: [130]}[tid]
            for part_id, album in zip(part_ids, entry['albums']):
                old = c.execute('SELECT name FROM parts WHERE id=? AND title_id=?', (part_id, tid)).fetchone()
                assert old
                replacements[old[0]] = album['name']
                c.execute('UPDATE parts SET name=?,release_date=? WHERE id=?', (album['name'], album['release_date'], part_id))
                for iid, numbers in c.execute('SELECT id,numbers FROM items WHERE part_id=?', (part_id,)).fetchall():
                    number = json.loads(numbers)['track']
                    assert 0 < number <= len(album['tracks'])
                    c.execute('UPDATE items SET name=? WHERE id=?', (album['tracks'][number-1], iid))
    for rid, name in c.execute('SELECT id,name FROM releases').fetchall():
        renamed = name
        for old, new in replacements.items():
            renamed = renamed.replace(old, new)
        # Fixture release years must agree with the real album/show names.
        for old, new in [('Discovery (2019)', 'Discovery (2001)'), ('Random Access Memories (2022)', 'Random Access Memories (2013)'), ('Dawn FM (2020)', 'Dawn FM (2022)')]:
            renamed = renamed.replace(old, new)
        if renamed != name:
            c.execute('UPDATE releases SET name=? WHERE id=?', (renamed, rid))
    assert before == c.execute('SELECT * FROM downloads ORDER BY id').fetchall(), 'Download lifecycle changed'
    assert slots == c.execute('SELECT * FROM slots ORDER BY id').fetchall(), 'Slot lifecycle changed'
    assert imports == c.execute('SELECT * FROM imports ORDER BY id').fetchall(), 'Import evidence changed'
    c.commit()
    print('Real metadata applied to five preview titles; downloads, slots and imports unchanged.')
except BaseException:
    c.rollback()
    raise
finally:
    c.close()
