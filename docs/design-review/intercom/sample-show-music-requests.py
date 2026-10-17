"""Seed six disposable request fixtures; approve/deny through the normal API."""
import datetime
import json
import sqlite3
import tempfile
import urllib.request
from pathlib import Path

BASE = 'http://127.0.0.1:18747'
DB = Path(tempfile.gettempdir()) / 'holyarrs-intercom-preview-18747/config/holyarrs.db'
catalog = json.loads((Path(__file__).parent / 'sample-catalog.json').read_text(encoding='utf-8'))
targets = [('show', 'Breaking Bad', 'asked'), ('show', 'Game of Thrones', 'approved'),
           ('show', 'Rick and Morty', 'turned_down'), ('artist', 'Hozier', 'asked'),
           ('artist', 'Queen', 'approved'), ('artist', 'Fleetwood Mac', 'turned_down')]

def api(path, body=None, method=None):
    headers = {'Content-Type': 'application/json'}
    if csrf:
        headers['X-CSRF-Token'] = csrf
    req = urllib.request.Request(BASE+path, data=None if body is None else json.dumps(body).encode(),
                                 headers=headers, method=method or ('GET' if body is None else 'POST'))
    with urllib.request.urlopen(req, timeout=20) as response:
        return json.load(response)

csrf = ''
me = api('/api/v1/auth/me')
assert me['role'] == 'owner'
csrf = me.get('csrf', '')
assert DB.is_file()
c = sqlite3.connect(DB, timeout=5)
c.row_factory = sqlite3.Row
try:
    c.execute('BEGIN IMMEDIATE')
    assert c.execute('SELECT url FROM sources WHERE id=1').fetchone()[0] == 'http://127.0.0.1:28767'
    prior_downloads = [tuple(r) for r in c.execute('SELECT * FROM downloads ORDER BY id')]
    previous = [tuple(r) for r in c.execute('SELECT * FROM asks ORDER BY id')]
    previous_ids = [r[0] for r in previous]
    actor = c.execute('SELECT asked_by FROM asks ORDER BY id LIMIT 1').fetchone()[0]
    now = datetime.datetime.now(datetime.timezone.utc).isoformat().replace('+00:00', 'Z')
    who = json.dumps({'kind': 'user', 'id': actor})
    records = []
    for kind, name, state in targets:
        entry = next(x for x in catalog['shows' if kind == 'show' else 'artists'] if x['name'] == name)
        existing = c.execute('SELECT id,state FROM asks WHERE kind=? AND name=?', (kind, name)).fetchall()
        assert len(existing) <= 1, 'Ambiguous preview request'
        if existing:
            records.append((existing[0]['id'], kind, name, state))
            continue
        title = c.execute('SELECT id,source_ids FROM titles WHERE kind=? AND name=? AND state<>?', (kind, name, 'removed')).fetchone()
        assert title, 'Missing preview catalog title: '+name
        ids = json.loads(title['source_ids'])
        seasons, albums = [], []
        if kind == 'show':
            seasons = [1]
            ref = 'tmdb:'+str(entry['tmdb'])
            year = entry.get('year')
            tmdb = str(entry['tmdb'])
        else:
            album = entry['albums'][0]
            albums = [{'mbid': album['id'], 'name': album['name'], 'year': int(album['release_date'][:4])}]
            ref, year, tmdb = 'mbid:'+entry['id'], None, ''
        assert entry['artwork'].startswith('https://')
        cur = c.execute('''INSERT INTO asks
            (created_at,created_by,updated_at,updated_by,kind,tmdb_id,name,year,poster,state,
             asked_by,asked_at,note,auto_approved,ref,source_ids,seasons,albums)
            VALUES (?,?,?,?,?,?,?,?,?,'asked',?,?,?,0,?,?,?,?)''',
            (now,who,now,who,kind,tmdb,name,year,entry['artwork'],actor,now,'Requested for the preview household.',ref,
             json.dumps(ids),json.dumps(seasons),json.dumps(albums)))
        c.execute('INSERT INTO ask_people (ask_id,user_id,asked_at,counted) VALUES (?,?,?,0)', (cur.lastrowid,actor,now))
        records.append((cur.lastrowid,kind,name,state))
    assert prior_downloads == [tuple(r) for r in c.execute('SELECT * FROM downloads ORDER BY id')]
    assert previous == [tuple(r) for r in c.execute('SELECT * FROM asks WHERE id IN ('+','.join('?' for _ in previous_ids)+') ORDER BY id', previous_ids)]
    c.commit()
except BaseException:
    c.rollback()
    raise
finally:
    c.close()

# Pending initialization is a fixture-only DB operation because the local owner
# automatically approves normal request creation. No permissions/auth are changed.
settings = api('/api/v1/settings/asks')
try:
    api('/api/v1/settings/asks', {**settings, 'search_on_approve': False}, 'PUT')
    current = {a['id']: a for a in api('/api/v1/asks?limit=200')['asks']}
    for aid, kind, name, state in records:
        if current[aid]['state'] == state:
            continue
        assert current[aid]['state'] == 'asked', 'Unexpected fixture request state'
        if state != 'asked':
            action = 'approve' if state == 'approved' else 'turn-down'
            api(f'/api/v1/asks/{aid}/{action}', {'note': 'Approved for the preview household.' if state == 'approved' else 'Not planned for the preview household right now.'})
finally:
    api('/api/v1/settings/asks', settings, 'PUT')
assert api('/api/v1/settings/asks') == settings
final = {a['id']: a for a in api('/api/v1/asks?limit=200')['asks']}
for aid, kind, name, state in records:
    assert final[aid]['state'] == state
print(json.dumps({'requests': [{'id': aid, 'kind': kind, 'name': name, 'state': state} for aid,kind,name,state in records], 'settings_restored': True}, indent=2))
