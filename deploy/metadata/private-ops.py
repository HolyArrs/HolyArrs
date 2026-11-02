"""Operator for this isolated private validation only; never publishes ports."""
import argparse
import json
import os
import pathlib
import subprocess
import time
import urllib.request

ROOT = pathlib.Path('/home/home/holyarrs-metadata-server')
RELEASE_FOLDERS = {
    f'20261015{suffix}': f'2026-10-15-build-{number}'
    for number, suffix in enumerate('abcdefgh', start=1)
}
EXISTING = ['holycore', 'sacris', 'cloudflared', 'watchtower', 'portainer_agent', 'buildx_buildkit_cw-pretag150-arm-node']
API = 'holyarrs-metadata-private'
FIXTURE = 'holyarrs-metadata-private-fixture'
NETWORK = 'holyarrs-metadata-private'

def release_folder(release):
    try:
        return RELEASE_FOLDERS[release]
    except KeyError as error:
        raise RuntimeError('unknown release') from error

def release_paths(release):
    folder = release_folder(release)
    return {
        'evidence': ROOT/'results'/'history'/folder,
        'data': ROOT/'data'/'test-runs'/folder,
        'credentials': ROOT/'settings'/'private'/folder,
        'config': ROOT/'settings'/'versions'/folder,
        'snapshots': ROOT/'backups'/'database-copies'/folder,
    }

def command(args, check=True):
    result = subprocess.run(args, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if check and result.returncode:
        raise RuntimeError('private operation failed: ' + args[0] + ' ' + args[1])
    return result

def inspect(name):
    # Select only nonsecret fields; never emit full inspection.
    expression = '{{json .State}}'
    state = json.loads(command(['docker', 'container', 'inspect', '--format', expression, name]).stdout)
    return {'name': name, 'id': command(['docker', 'container', 'inspect', '--format', '{{.Id}}', name]).stdout.strip(), 'started': state['StartedAt'], 'running': state['Running'], 'health': state.get('Health', {}).get('Status'), 'restarts': int(command(['docker','container','inspect','--format','{{.RestartCount}}',name]).stdout)}

def existing():
    return [inspect(name) for name in EXISTING]

def save(path, value):
    path.write_text(json.dumps(value, indent=2, sort_keys=True)+'\n')

def ip(name):
    return command(['docker','container','inspect','--format','{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}',name]).stdout.strip()

def owned(name):
    found=command(['docker','container','inspect','--format','{{index .Config.Labels "holyforge.owner"}}',name])
    if found.stdout.strip()!='holyarrs-metadata-private':raise RuntimeError('container ownership mismatch')

def limits(name, cpus, memory):
    return ['--pull=never','--name',name,'--restart','no','--cpus',str(cpus),'--memory',memory,'--pids-limit','128','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--tmpfs','/tmp:rw,noexec,nosuid,size=16m','--label','com.centurylinklabs.watchtower.enable=false','--label','holyforge.owner=holyarrs-metadata-private','--log-driver','json-file','--log-opt','max-size=10m','--log-opt','max-file=3']

def wait_ready(address, timeout=60):
    deadline=time.monotonic()+timeout
    while time.monotonic()<deadline:
        try:
            with urllib.request.urlopen('http://'+address+':8080/readyz',timeout=2) as reply:
                if reply.status==200:return
        except (OSError,ValueError):pass
        time.sleep(.2)
    raise RuntimeError('private API did not become ready')

def main():
    p=argparse.ArgumentParser();p.add_argument('action',choices=['baseline','setup','fast','paced','quota','start','stop','status','snapshot']);p.add_argument('--release',required=True);a=p.parse_args()
    if not a.release or any(c not in 'abcdefghijklmnopqrstuvwxyz0123456789-' for c in a.release):raise RuntimeError('invalid release')
    paths=release_paths(a.release)
    # Emergency stop must work even when disk, build evidence or unrelated services fail.
    if a.action=='stop':
        for name in [API,FIXTURE]:
            if command(['docker','container','inspect','--format','{{.Id}}',name],False).returncode:
                continue
            owned(name)
            command(['docker','stop','--time','25',name])
        print(json.dumps({'action':'stop','owned_containers_stopped':True}))
        return
    if a.action=='status':
        result={'action':'status','existing':existing(),'available_bytes':os.statvfs('/').f_bavail*os.statvfs('/').f_frsize}
        for name in [API,FIXTURE]:
            result[name]=inspect(name) if command(['docker','container','inspect','--format','{{.Id}}',name],False).returncode==0 else None
        print(json.dumps(result))
        return
    evidence=paths['evidence'];evidence.mkdir(parents=True,exist_ok=True)
    data_dir=paths['data']
    credential_dir=paths['credentials']
    config_dir=paths['config']
    snapshot_dir=paths['snapshots']
    image='holyarrs-metadata:private-'+a.release
    fixture_image='holyarrs-metadata-fixture:private-'+a.release
    if a.action=='baseline':
        if (evidence/'existing-before.json').exists():raise RuntimeError('baseline already frozen')
        save(evidence/'existing-before.json',existing());return
    identities=(evidence/'image-identities.txt').read_text().splitlines()
    expected=[line.split()[0] for line in identities]
    for tag,identity in zip([image,fixture_image],expected):
        if command(['docker','image','inspect','--format','{{.Id}} {{.Architecture}}',tag]).stdout.strip()!=identity+' arm64':raise RuntimeError('image identity mismatch')
    image,fixture_image=expected
    baseline=json.loads((evidence/'existing-before.json').read_text())
    now=existing()
    if any(x['id']!=y['id'] or x['started']!=y['started'] or x['restarts']!=y['restarts'] or not y['running'] or (x['health']=='healthy' and y['health']!='healthy') for x,y in zip(baseline,now)):
        save(evidence/'existing-change.json',now);raise RuntimeError('existing service state changed; validation halted')
    if os.statvfs('/').f_bavail*os.statvfs('/').f_frsize < 25*1024**3:raise RuntimeError('disk reserve below 25 GiB')
    if a.action=='setup':
        if any(folder.exists() for folder in [data_dir,credential_dir,config_dir,snapshot_dir]):raise RuntimeError('release directory already exists')
        for folder in [data_dir,credential_dir,config_dir,snapshot_dir]:
            folder.mkdir(parents=True,mode=0o700,exist_ok=True);os.chown(folder,10001,10001)
        cfg={'listen':':8080','data_dir':'/data','private_fixtures':True,'providers':{'tmdb':{'base_url':'http://fixture:8081/tmdb/3','interval_ms':200},'tvmaze':{'base_url':'http://fixture:8081/tvmaze','interval_ms':1000},'musicbrainz':{'base_url':'http://fixture:8081/musicbrainz/ws/2','interval_ms':1100}},'cache_bytes':2147483648,'cache_entries':100000,'min_free_bytes':21474836480,'ttl_seconds':86400,'stale_seconds':0,'per_minute':60000,'burst':1000,'per_token_concurrent':100,'max_concurrent':100,'max_pending':200}
        save(config_dir/'private.json',cfg);os.chmod(config_dir/'private.json',0o644)
        command(['docker','network','create','--internal','--label','holyforge.owner=holyarrs-metadata-private',NETWORK])
        command(['docker','run','-d']+limits(FIXTURE,.25,'256m')+['--network',NETWORK,'--network-alias','fixture',fixture_image])
        issued=command(['docker','run','--rm']+limits('holyarrs-metadata-private-issue',.5,'512m')+['--network','none','-v',str(data_dir)+':/data','-v',str(credential_dir)+':/credentials',image,'token-issue','--data-dir','/data','--token-file','/credentials/token'])
        save(evidence/'credential-issued.json',{'exit':issued.returncode,'private_file':True})
        # Host runner reads its own newly generated private credential; never print it.
        os.chmod(credential_dir/'token',0o600)
    if a.action in ('fast','paced','quota'):
        cfg=json.loads((config_dir/'private.json').read_text())
        values={'tmdb':5,'tvmaze':5,'musicbrainz':5} if a.action=='fast' else {'tmdb':200,'tvmaze':1000,'musicbrainz':1100}
        for provider,value in values.items():cfg['providers'][provider]['interval_ms']=value
        cfg['per_minute']=60 if a.action=='quota' else 60000
        cfg['burst']=10 if a.action=='quota' else 1000
        cfg['per_token_concurrent']=4 if a.action=='quota' else 100
        save(config_dir/'private.json',cfg)
    if a.action in ('start','fast','paced','quota'):
        found=command(['docker','container','inspect','--format','{{index .Config.Labels "holyforge.owner"}}',API],check=False)
        if found.returncode==0:
            if found.stdout.strip()!='holyarrs-metadata-private':raise RuntimeError('container ownership mismatch')
            command(['docker','stop','--time','25',API]);command(['docker','rm',API])
        command(['docker','run','-d']+limits(API,1,'1g')+['--network',NETWORK,'-v',str(data_dir)+':/data','-v',str(config_dir)+':/config:ro',image,'serve','--config','/config/private.json'])
        wait_ready(ip(API))
    if a.action=='snapshot':
        owned(API)
        command(['docker','stop','--time','25',API])
        command(['docker','run','--rm']+limits('holyarrs-metadata-private-backup',.5,'512m')+['--network','none','-v',str(data_dir)+':/data','-v',str(snapshot_dir)+':/backup','-v',str(config_dir)+':/config:ro',image,'backup','--config','/config/private.json','--data-dir','/data','--destination','/backup/initial'])
    api_exists=command(['docker','container','inspect','--format','{{.Id}}',API],False).returncode==0
    fixture_exists=command(['docker','container','inspect','--format','{{.Id}}',FIXTURE],False).returncode==0
    status={'existing':existing(),'available_bytes':os.statvfs('/').f_bavail*os.statvfs('/').f_frsize,'api':inspect(API) if api_exists else None,'fixture':inspect(FIXTURE) if fixture_exists else None,'api_ip':ip(API) if api_exists else None,'fixture_ip':ip(FIXTURE) if fixture_exists else None}
    save(evidence/'status-latest.json',status)
    print(json.dumps({'action':a.action,'existing_unchanged':status['existing']==baseline,'available_bytes':status['available_bytes'],'api_ip':status['api_ip'],'fixture_ip':status['fixture_ip']}))

if __name__=='__main__':main()
