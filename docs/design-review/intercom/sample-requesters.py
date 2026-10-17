"""Add non-login sample request participants only to the disposable preview."""
import datetime
import json
import sqlite3
import tempfile
from pathlib import Path

DB = Path(tempfile.gettempdir()) / 'holyarrs-intercom-preview-18747/config/holyarrs.db'
assert DB.is_file()
c = sqlite3.connect(DB, timeout=5)
try:
    c.execute('BEGIN IMMEDIATE')
    assert c.execute('SELECT url FROM sources WHERE id=1').fetchone() == ('http://127.0.0.1:28767',)
    lifecycle = {table: c.execute(f'SELECT * FROM {table} ORDER BY id').fetchall() for table in ('downloads','slots','imports')}
    owner = c.execute("SELECT id FROM users WHERE role='owner'").fetchone()[0]
    now = datetime.datetime.now(datetime.timezone.utc).isoformat().replace('+00:00','Z')
    actor = json.dumps({'kind':'user','id':owner})
    # Disabled members with no credentials or permissions; existing users untouched.
    assignments = {'Maya (sample)': [22,21,1], 'Alex (sample)': [25,24,2], 'Jordan (sample)': [13,20,23]}
    ids = {}
    for name, asks in assignments.items():
        row = c.execute('SELECT id,state,role FROM users WHERE name=?',(name,)).fetchone()
        if row:
            assert row[1:] == ('disabled','member')
            uid = row[0]
        else:
            uid = c.execute("INSERT INTO users (created_at,created_by,updated_at,updated_by,name,role,password_hash,state,perms) VALUES (?,?,?,?,?,'member','','disabled','[]')",(now,actor,now,actor,name)).lastrowid
        ids[name] = uid
        for aid in asks:
            asked_at = c.execute('SELECT asked_at FROM asks WHERE id=?',(aid,)).fetchone()[0]
            c.execute('UPDATE asks SET asked_by=? WHERE id=?',(uid,aid))
            c.execute('DELETE FROM ask_people WHERE ask_id=? AND user_id=?',(aid,owner))
            c.execute('INSERT OR IGNORE INTO ask_people (ask_id,user_id,asked_at,counted) VALUES (?,?,?,0)',(aid,uid,asked_at))
    # A shared TV request demonstrates one title appearing under both people.
    c.execute('INSERT OR IGNORE INTO ask_people (ask_id,user_id,asked_at,counted) VALUES (22,?,?,0)',(ids['Alex (sample)'],now))
    for table, rows in lifecycle.items():
        assert rows == c.execute(f'SELECT * FROM {table} ORDER BY id').fetchall()
    c.commit()
    print('Added Maya, Alex and Jordan sample requesters, including shared Game of Thrones. Existing credentials and media lifecycle unchanged.')
except BaseException:
    c.rollback()
    raise
finally:
    c.close()
