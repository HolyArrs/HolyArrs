import importlib.util
import json
import pathlib
import subprocess
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('private_ops',pathlib.Path(__file__).with_name('private-ops.py'))
ops=importlib.util.module_from_spec(spec);spec.loader.exec_module(ops)

class PrivateOperatorTests(unittest.TestCase):
    def test_emergency_stop_needs_only_owned_container_inspection(self):
        with patch('sys.argv',['private-ops.py','stop','--release','20261015g']), patch.object(ops,'command',return_value=subprocess.CompletedProcess([],0,'identity','')) as commands, patch.object(ops,'owned') as owned, patch.object(ops.os,'statvfs',side_effect=AssertionError('stop checked free space'),create=True):
            ops.main()
            self.assertEqual(owned.call_count,2)
            self.assertEqual(sum(args.args[0][:2]==['docker','stop'] for args in commands.call_args_list),2)

    def test_read_only_status_works_without_image_or_disk_evidence(self):
        with patch('sys.argv',['private-ops.py','status','--release','20261015g']), patch.object(ops,'existing',return_value=[]), patch.object(ops,'command',return_value=subprocess.CompletedProcess([],1,'','')), patch.object(ops.os,'statvfs',create=True) as filesystem:
            filesystem.return_value.f_bavail=1;filesystem.return_value.f_frsize=4096
            ops.main()

    def test_inspection_selects_containers_not_same_named_network(self):
        state={'StartedAt':'fixed','Running':True}
        responses=[json.dumps(state),'identity','0']
        with patch.object(ops,'command',side_effect=[subprocess.CompletedProcess([],0,x,'') for x in responses]) as call:
            self.assertTrue(ops.inspect('private-test')['running'])
            for args in call.call_args_list:self.assertEqual(args.args[0][:3],['docker','container','inspect'])

    def test_unowned_container_cannot_be_stopped(self):
        with patch.object(ops,'command',return_value=subprocess.CompletedProcess([],0,'another-owner','')):
            with self.assertRaises(RuntimeError):ops.owned('private-test')

    def test_runtime_limits_never_publish_ports_or_enable_restart(self):
        args=ops.limits('private-test',1,'1g')
        self.assertNotIn('-p',args);self.assertNotIn('--publish',args)
        self.assertEqual(args[args.index('--restart')+1],'no')
        self.assertIn('--pull=never',args);self.assertIn('--read-only',args)

    def test_release_paths_use_the_five_folder_layout(self):
        paths=ops.release_paths('20261015g')
        expected_root=pathlib.Path('/home/home/holyarrs-metadata-server')
        self.assertEqual(paths['evidence'],expected_root/'results'/'history'/'2026-10-15-build-7')
        self.assertEqual(paths['data'],expected_root/'data'/'test-runs'/'2026-10-15-build-7')
        self.assertEqual(paths['credentials'],expected_root/'settings'/'private'/'2026-10-15-build-7')
        self.assertEqual(paths['config'],expected_root/'settings'/'versions'/'2026-10-15-build-7')
        self.assertEqual(paths['snapshots'],expected_root/'backups'/'database-copies'/'2026-10-15-build-7')
        self.assertFalse(any('/var/opt/' in str(path) or '/etc/opt/' in str(path) for path in paths.values()))
        self.assertEqual(ops.release_folder('20261015h'),'2026-10-15-build-8')

    def test_operator_and_build_script_do_not_use_legacy_storage_roots(self):
        folder=pathlib.Path(__file__).parent
        text=(folder/'private-ops.py').read_text()+(folder/'private-build.sh').read_text()
        for legacy_root in ['/opt/holyforge/holyarrs-metadata','/etc/opt/holyforge/holyarrs-metadata','/var/opt/holyforge/holyarrs-metadata']:
            self.assertNotIn(legacy_root,text)

    def test_unknown_release_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError,'unknown release'):
            ops.release_paths('20261015z')

class ReadinessTests(unittest.TestCase):
    def test_wait_allows_integrity_check_beyond_old_ten_seconds(self):
        response=unittest.mock.MagicMock();response.__enter__.return_value.status=200
        with patch.object(ops.time,'monotonic',side_effect=[0,0,11,22]), patch.object(ops.time,'sleep'), patch.object(ops.urllib.request,'urlopen',side_effect=[OSError(),OSError(),response]):
            ops.wait_ready('private-test')

    def test_wait_stops_at_deadline(self):
        with patch.object(ops.time,'monotonic',side_effect=[0,0,61]), patch.object(ops.time,'sleep'), patch.object(ops.urllib.request,'urlopen',side_effect=OSError()):
            with self.assertRaises(RuntimeError):ops.wait_ready('private-test')

if __name__=='__main__':unittest.main()

