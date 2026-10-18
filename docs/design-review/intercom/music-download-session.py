"""Create/revoke a new disposable admin session; never update existing users."""
import hashlib
import json
from pathlib import Path
import sqlite3
import sys
import tempfile
from datetime import datetime, timedelta, timezone

v = json.load(sys.stdin)
db = Path(tempfile.gettempdir()) / 'holyarrs-intercom-preview-18747/config/holyarrs.db'
c = sqlite3.connect(db)
def stamp(t):
    return t.strftime('%Y-%m-%dT%H:%M:%S.') + f'{t.microsecond:06d}000Z'
now = datetime.now(timezone.utc)
name = v['name']
assert name.startswith('music-download-') and len(name) == 23
actor = json.dumps({'kind':'system'})
with c:
    if v['action'] == 'create':
        assert c.execute('SELECT count(*) FROM users WHERE name=?',(name,)).fetchone()[0] == 0
        cur=c.execute("INSERT INTO users(created_at,created_by,updated_at,updated_by,name,role,password_hash,state,perms) VALUES(?,?,?,?,?,'admin','','active',?)", (stamp(now),actor,stamp(now),actor,name,json.dumps(['manage_library','settings','ask'])))
        uid=cur.lastrowid
        c.execute("INSERT INTO sessions(hash,user_id,csrf,created_at,rotated_at,last_seen_at,expires_at,user_agent,family,auth_method,local_auth_at) VALUES(?,?,?,?,?,?,?,?,?,'local_password',?)",(hashlib.sha256(v['token'].encode()).hexdigest(),uid,v['csrf'],stamp(now),stamp(now),stamp(now),stamp(now+timedelta(minutes=20)),'Disposable music import check',name,stamp(now)))
    elif v['action'] == 'revoke':
        row=c.execute("SELECT id,password_hash FROM users WHERE name=? AND role='admin'",(name,)).fetchone()
        assert row and row[1] == ''
        uid=row[0]
        c.execute('DELETE FROM sessions WHERE user_id=?',(uid,))
        c.execute("UPDATE users SET state='disabled',updated_at=?,updated_by=? WHERE id=? AND name=?",(stamp(now),actor,uid,name))
    else:
        raise ValueError('Unknown action')
print(json.dumps({'id':uid,'action':v['action']}))
