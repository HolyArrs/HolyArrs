"""Persist approved-request artwork in the disposable preview Library only."""
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
    before = {table: c.execute(f'SELECT * FROM {table} ORDER BY id').fetchall() for table in ('asks', 'downloads', 'slots', 'imports')}
    for aid, tid, kind, name in [(22, 76, 'show', 'Game of Thrones'), (25, 141, 'artist', 'Queen')]:
        ask = c.execute('SELECT state,title_id,poster FROM asks WHERE id=? AND kind=? AND name=?', (aid,kind,name)).fetchone()
        assert ask and ask[0] == 'approved' and ask[1] == tid and ask[2].startswith('https://')
        assert c.execute('SELECT state FROM titles WHERE id=? AND kind=? AND name=?', (tid,kind,name)).fetchone() == ('active',)
        c.execute('UPDATE titles SET artwork=? WHERE id=?', (json.dumps([ask[2]]),tid))
    for table, rows in before.items():
        assert rows == c.execute(f'SELECT * FROM {table} ORDER BY id').fetchall()
    c.commit()
    print('Approved Library links verified; artwork persisted. Requests and media lifecycle unchanged.')
except BaseException:
    c.rollback()
    raise
finally:
    c.close()
