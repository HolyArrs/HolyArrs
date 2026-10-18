"""Read-only fingerprints for the retained disposable preview music journey."""
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import tempfile

root = Path(tempfile.gettempdir()) / 'holyarrs-intercom-preview-18747'
media = Path(tempfile.gettempdir()) / 'holyarrs-intercom-preview-media-18747'
c = sqlite3.connect((root / 'config/holyarrs.db').as_uri() + '?mode=ro', uri=True)
c.row_factory = sqlite3.Row
def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, default=str).encode()).hexdigest()
result = {'tables': {}, 'files': {}}
for table in ('users', 'api_keys', 'secrets', 'settings', 'invites', 'ask_people',
              'library_folders', 'downloaders', 'pick_rules', 'user_links'):
    rows = [dict(r) for r in c.execute('SELECT * FROM ' + table)]
    if table == 'downloaders':
        for r in rows:
            for key in ('updated_at', 'updated_by', 'checked_at', 'last_ok_at', 'last_error', 'state', 'failures', 'backoff_until'):
                r.pop(key, None)
    if table == 'downloaders':
        result['original_downloader'] = digest(sorted([r for r in rows if r['id'] == 1], key=lambda r: json.dumps(r, sort_keys=True, default=str)))
    result['tables'][table] = digest(sorted(rows, key=lambda r: json.dumps(r, sort_keys=True, default=str)))
for table, condition in [('asks', 'id != 34'), ('titles', 'id != 166'),
                         ('slots', 'id NOT IN (1183,1184,1185)'), ('files', '1=1')]:
    result['file_rows' if table == 'files' else table] = {str(r['id']): digest(dict(r)) for r in c.execute('SELECT * FROM ' + table + ' WHERE ' + condition + ' ORDER BY id')}
for table, columns in [('downloads', 'id,created_at,created_by,downloader_id,release_id,external_id,save_path,started_by'),
                       ('imports', 'id,created_at,created_by,download_id,from_path,kind,mode')]:
    result[table] = {str(r['id']): digest(dict(r)) for r in c.execute('SELECT ' + columns + ' FROM ' + table)}
for base in (root / 'downloads', root / 'movies', media):
    if not base.exists():
        continue
    for current, dirs, files in os.walk(base, followlinks=False):
        for name in dirs + files:
            p = Path(current) / name
            if p.is_symlink() or (hasattr(p, 'is_junction') and p.is_junction()):
                raise RuntimeError('Reparse path in disposable storage')
        for name in files:
            p = Path(current) / name
            result['files'][str(p)] = hashlib.sha256(p.read_bytes()).hexdigest()
print(json.dumps(result))
