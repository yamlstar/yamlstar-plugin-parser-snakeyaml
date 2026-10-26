import contextlib
import importlib.machinery
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import struct
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[1]
loader = importlib.machinery.SourceFileLoader(
    'plugin_release', str(ROOT / 'util/release'))
spec = importlib.util.spec_from_loader(loader.name, loader)
module = importlib.util.module_from_spec(spec)
loader.exec_module(module)
HEAD = '1' * 40
VERSION = '0.1.0'


def make_archives(directory, wrong_architecture=False):
    directory.mkdir()
    for platform in module.PLATFORMS:
        root = f'{module.ARTIFACT}-{VERSION}-{platform}'
        extension = '.dylib' if platform.startswith('macos') else '.so'
        if platform.startswith('linux'):
            header = bytearray(20)
            header[:6] = b'\x7fELF\x02\x01'
            struct.pack_into('<H', header, 18,
                             62 if platform == 'linux-x64' or wrong_architecture
                             else 183)
        else:
            header = struct.pack('<II', 0xfeedfacf, 0x0100000c)
        files = {
            'plugin.edn': b'{:abi 2 :name "snakeyaml" :version "0.1.0"}',
            'License': b'MIT',
            'licenses/snakeyaml-engine.txt': b'Apache 2.0',
            'lib/lib' + module.ARTIFACT + extension: bytes(header),
        }
        with tarfile.open(directory / (root + '.tar.xz'), 'w:xz') as archive:
            for name, data in files.items():
                entry = tarfile.TarInfo(root + '/' + name)
                entry.size = len(data)
                archive.addfile(entry, io.BytesIO(data))
    write_checksums(directory)


def write_checksums(directory):
    lines = [module.hashlib.sha256(path.read_bytes()).hexdigest() + '  ' + path.name
             for path in sorted(directory.glob('*.tar.xz'))]
    (directory / 'SHA256SUMS').write_text('\n'.join(lines) + '\n')


class Services:
    """Model GitHub and Central while keeping all publication calls offline."""

    def __init__(self, fixtures):
        self.fixtures = fixtures
        self.references = {}
        self.tag_objects = {}
        self.release = None
        self.uploaded = {}
        self.central = False
        self.dirty = False
        self.remote_head = HEAD
        self.commit_error = None
        self.fail = None
        self.corrupt_download = False
        self.edit_during_download = False
        self.missing_once = set()
        self.events = []
        self.workflow_run = {'id': 20, 'head_sha': HEAD,
                             'path': '.github/workflows/release.yaml'}
        self.dispatch_arguments = None
        self.release_runs = []

    def command(self, *args, payload=None, stream=False):
        if args[:3] == ('git', 'status', '--porcelain'):
            return ' M pom.xml' if self.dirty else ''
        if args[:3] == ('git', 'rev-parse', 'HEAD'):
            return HEAD
        if args[:3] == ('git', 'branch', '--show-current'):
            return 'main'
        if args[1] == 'util/check-versions':
            return ''
        if args[1] == 'util/maven-release':
            self.events.append('central-publish')
            if self.fail == 'central':
                raise module.ReleaseError('Central validation failed')
            self.central = True
            return ''
        if args[0] != 'fixture-gh':
            raise AssertionError(args)
        if args[1] == 'api':
            endpoint = args[2].removeprefix('repos/' + module.REPOSITORY + '/')
            return json.dumps(self.api(endpoint, payload))
        args = args[1:-2]
        if args[:2] == ('workflow', 'run'):
            self.events.append('dispatch-' + args[2])
            self.dispatch_arguments = args
        elif args[:2] == ('run', 'rerun'):
            self.events.append('rerun-' + args[2])
            if '--failed' not in args:
                raise AssertionError(args)
        elif args[:2] == ('run', 'download'):
            if self.fail == 'artifact':
                raise module.ReleaseError('Artifact expired')
            destination = Path(args[args.index('--dir') + 1])
            shutil.copytree(self.fixtures, destination)
            if self.edit_during_download:
                self.dirty = True
        elif args[:2] == ('release', 'create'):
            self.events.append('github-draft')
            self.release = {'tag_name': VERSION, 'draft': True,
                            'target_commitish': HEAD}
        elif args[:2] == ('release', 'upload'):
            self.events.append('github-upload')
            if self.fail == 'upload':
                raise module.ReleaseError('GitHub upload failed')
            self.uploaded = {Path(name).name: Path(name).read_bytes()
                             for name in args[4:]}
        elif args[:2] == ('release', 'edit'):
            self.events.append('github-publish')
            if self.fail == 'github':
                raise module.ReleaseError('GitHub publication failed')
            self.release['draft'] = False
            self.references[VERSION] = {'type': 'commit', 'sha': HEAD}
        else:
            raise AssertionError(args)
        return ''

    def api(self, endpoint, payload):
        if endpoint.startswith('commits/'):
            if endpoint.endswith(HEAD) and self.commit_error:
                raise module.ReleaseError(self.commit_error)
            ref = endpoint.split('/')[-1]
            return {'sha': ref if len(ref) == 40 else self.remote_head}
        if endpoint.startswith('git/matching-refs/tags/'):
            tag = endpoint.split('/')[-1]
            return ([{'ref': 'refs/tags/' + tag, 'object': self.references[tag]}]
                    if tag in self.references else [])
        if endpoint.startswith('git/tags/'):
            return {'object': self.tag_objects[endpoint.split('/')[-1]]}
        if endpoint.startswith('actions/runs/'):
            if endpoint != 'actions/runs/20':
                raise AssertionError(endpoint)
            return self.workflow_run
        if endpoint.startswith('actions/workflows/release.yaml/runs?'):
            return {'workflow_runs': self.release_runs}
        if endpoint.startswith('releases?'):
            return [self.release] if self.release else []
        if endpoint == 'git/refs':
            self.events.append('go-tag')
            if self.fail == 'tag':
                raise module.ReleaseError('Go tag publication failed')
            if payload != {'ref': 'refs/tags/v' + VERSION, 'sha': HEAD}:
                raise AssertionError(payload)
            self.references['v' + VERSION] = {'type': 'commit', 'sha': HEAD}
            return {'ref': payload['ref']}
        raise AssertionError(endpoint)

    def fetch(self, url):
        if 'repo.maven.apache.org' in url:
            self.events.append('central-download')
            if url in self.missing_once:
                self.missing_once.remove(url)
                return None
            return b'published Java artifact' if self.central else None
        self.events.append('native-download')
        if self.release is None or self.release['draft']:
            return None
        if self.corrupt_download:
            return b'different release artifact'
        return self.uploaded[url.rsplit('/', 1)[-1]]


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.directory = Path(temporary.name)
        self.fixtures = self.directory / 'fixtures'
        make_archives(self.fixtures)
        previous = Path.cwd()
        os.chdir(self.directory)
        self.addCleanup(os.chdir, previous)
        environment = patch.dict(os.environ, {
            'GH': 'fixture-gh', 'GH_TOKEN': 'fixture-private-token',
            'PERL': 'fixture-perl', 'MAVEN': 'fixture-maven',
            'MAVEN_REPOSITORY': '/fixture/m2',
            'GITHUB_ACTIONS': 'true', 'GITHUB_RUN_ID': '20',
            'GITHUB_REPOSITORY': module.REPOSITORY,
            'GITHUB_WORKFLOW_REF': (
                module.REPOSITORY + '/.github/workflows/release.yaml@refs/heads/main'),
            'RELEASE_COMMIT': HEAD,
        }, clear=True)
        environment.start()
        self.addCleanup(environment.stop)
        self.release = module.Release(VERSION)
        self.services = Services(self.fixtures)
        self.release.command = self.services.command
        self.release.fetch = self.services.fetch
        sleep = patch.object(module.time, 'sleep')
        sleep.start()
        self.addCleanup(sleep.stop)

    def publish(self):
        with contextlib.redirect_stdout(io.StringIO()):
            self.release.publish()

    def assert_no_go_tag(self):
        self.assertNotIn('v' + VERSION, self.services.references)
        self.assertNotIn('go-tag', self.services.events)

    def test_complete_release_publishes_go_last(self):
        self.publish()
        events = self.services.events
        self.assertLess(events.index('github-upload'), events.index('central-publish'))
        self.assertLess(events.index('central-publish'),
                        events.index('github-publish'))
        self.assertEqual(events[-1], 'go-tag')
        self.assertEqual(events.count('native-download'), 4)
        self.assertEqual(set(self.services.uploaded), set(self.release.filenames))
        self.assertEqual(self.services.references['v' + VERSION]['sha'], HEAD)
        provenance = json.loads(Path(
            '.cache/release-assets/0.1.0-provenance.json').read_text())
        self.assertEqual(provenance['commit'], HEAD)
        self.assertEqual(provenance['run'], 20)

    def test_retry_reuses_published_artifacts(self):
        self.publish()
        self.services.events.clear()
        self.publish()
        for event in ('central-publish', 'github-draft', 'github-upload',
                      'github-publish', 'go-tag'):
            self.assertNotIn(event, self.services.events)

    def test_retry_after_go_tag_failure_does_not_republish(self):
        self.services.fail = 'tag'
        with self.assertRaisesRegex(module.ReleaseError, 'Go tag publication'):
            self.publish()
        self.assertNotIn('v' + VERSION, self.services.references)
        self.assertFalse(self.services.release['draft'])
        self.services.fail = None
        self.services.events.clear()
        self.publish()
        self.assertEqual(self.services.events[-1], 'go-tag')
        self.assertNotIn('central-publish', self.services.events)
        self.assertNotIn('github-publish', self.services.events)

    def test_retry_after_github_failure_finishes_without_redeploying(self):
        self.services.fail = 'github'
        with self.assertRaisesRegex(module.ReleaseError, 'GitHub publication'):
            self.publish()
        self.assert_no_go_tag()
        self.services.fail = None
        self.services.events.clear()
        self.publish()
        self.assertNotIn('central-publish', self.services.events)
        self.assertIn('go-tag', self.services.events)

    def test_missing_central_files_wait_without_redeploying(self):
        self.services.central = True
        self.services.missing_once.add(self.release.central_files()[-1])
        self.publish()
        self.assertNotIn('central-publish', self.services.events)

    def test_central_failure_keeps_github_draft_and_go_unpublished(self):
        self.services.fail = 'central'
        with self.assertRaisesRegex(module.ReleaseError, 'Central validation'):
            self.publish()
        self.assertTrue(self.services.release['draft'])
        self.assert_no_go_tag()

    def test_upload_failure_stops_before_central(self):
        self.services.fail = 'upload'
        with self.assertRaisesRegex(module.ReleaseError, 'GitHub upload'):
            self.publish()
        self.assertNotIn('central-publish', self.services.events)
        self.assert_no_go_tag()

    def test_public_native_mismatch_blocks_go_tag(self):
        self.services.corrupt_download = True
        with self.assertRaisesRegex(module.ReleaseError, 'differs from CI'):
            self.publish()
        self.assert_no_go_tag()

    def test_checksum_failure_stops_before_publishing(self):
        archive = next(self.fixtures.glob('*.tar.xz'))
        archive.write_bytes(b'corrupt archive')
        with self.assertRaisesRegex(module.ReleaseError, 'Checksum mismatch'):
            self.publish()
        self.assertFalse(self.services.events)
        self.assert_no_go_tag()

    def test_missing_platform_stops_before_publishing(self):
        next(self.fixtures.glob('*.tar.xz')).unlink()
        with self.assertRaisesRegex(module.ReleaseError, 'all three archives'):
            self.publish()
        self.assert_no_go_tag()

    def test_wrong_architecture_with_valid_checksum_is_rejected(self):
        shutil.rmtree(self.fixtures)
        make_archives(self.fixtures, wrong_architecture=True)
        with self.assertRaisesRegex(module.ReleaseError, 'Wrong native architecture'):
            self.publish()
        self.assert_no_go_tag()
        self.assertIsNone(self.services.release)

    def test_local_command_only_dispatches_release_workflow(self):
        del self.release.environment['MAVEN']
        del self.release.environment['MAVEN_REPOSITORY']
        with patch.object(module.sys, 'argv', ['util/release', VERSION]), \
                patch.object(module, 'Release', return_value=self.release), \
                contextlib.redirect_stdout(io.StringIO()) as output:
            module.main()
        self.assertEqual(self.services.events, ['dispatch-release.yaml'])
        self.assertIn('version=' + VERSION, self.services.dispatch_arguments)
        self.assertIn('commit=' + HEAD, self.services.dispatch_arguments)
        self.assertIn('/actions/workflows/release.yaml', output.getvalue())
        self.assertFalse(self.release.assets.exists())
        self.assert_no_go_tag()

    def test_local_dispatch_rejects_a_different_remote_branch_tip(self):
        self.services.remote_head = '2' * 40
        with self.assertRaisesRegex(module.ReleaseError, 'does not match HEAD'):
            self.release.dispatch()
        self.assert_no_go_tag()
        self.assertFalse(self.services.events)

    def test_local_command_reuses_an_active_workflow_without_waiting(self):
        self.services.release_runs = [{
            'id': 20, 'head_sha': HEAD, 'status': 'in_progress',
            'conclusion': None, 'html_url': 'https://github.com/fixture/runs/20',
        }]
        with contextlib.redirect_stdout(io.StringIO()) as output:
            self.release.dispatch()
        self.assertIn('already running', output.getvalue())
        self.assertFalse(self.services.events)

    def test_local_command_resumes_failed_jobs_without_waiting(self):
        self.services.release_runs = [{
            'id': 20, 'head_sha': HEAD, 'status': 'completed',
            'conclusion': 'failure', 'html_url': 'https://github.com/fixture/runs/20',
        }]
        with contextlib.redirect_stdout(io.StringIO()) as output:
            self.release.dispatch()
        self.assertIn('restarted', output.getvalue())
        self.assertEqual(self.services.events, ['rerun-20'])
        self.assert_no_go_tag()

    def test_local_command_does_not_restart_a_completed_release(self):
        self.services.release_runs = [{
            'id': 20, 'head_sha': HEAD, 'status': 'completed',
            'conclusion': 'success', 'html_url': 'https://github.com/fixture/runs/20',
        }]
        with contextlib.redirect_stdout(io.StringIO()) as output:
            self.release.dispatch()
        self.assertIn('already completed', output.getvalue())
        self.assertFalse(self.services.events)

    def test_publication_runs_only_in_actions(self):
        del self.release.environment['GITHUB_ACTIONS']
        with self.assertRaisesRegex(module.ReleaseError, 'Publication runs in'):
            self.publish()
        self.assertFalse(self.services.events)

    def test_publication_rejects_a_different_workflow(self):
        self.release.environment['GITHUB_WORKFLOW_REF'] = (
            module.REPOSITORY + '/.github/workflows/test.yaml@refs/heads/main')
        with self.assertRaisesRegex(module.ReleaseError, 'Publication runs in'):
            self.publish()
        self.assertFalse(self.services.events)

    def test_workflow_for_another_commit_is_not_accepted(self):
        self.services.workflow_run['head_sha'] = '2' * 40
        with self.assertRaisesRegex(module.ReleaseError,
                                    'does not match the reviewed'):
            self.publish()
        self.assert_no_go_tag()
        self.assertIsNone(self.services.release)

    def test_workflow_input_must_match_checkout(self):
        self.release.environment['RELEASE_COMMIT'] = '2' * 40
        with self.assertRaisesRegex(module.ReleaseError,
                                    'does not match the reviewed'):
            self.publish()
        self.assert_no_go_tag()

    def test_publish_cli_uses_actions_mode(self):
        arguments = ['util/release', '--publish', VERSION]
        with patch.object(module.sys, 'argv', arguments), \
                patch.object(module, 'Release', return_value=self.release), \
                contextlib.redirect_stdout(io.StringIO()):
            module.main()
        self.assertEqual(self.services.events[-1], 'go-tag')

    def test_missing_workflow_artifact_stops_before_publication(self):
        self.services.fail = 'artifact'
        with self.assertRaisesRegex(module.ReleaseError, 'Artifact expired'):
            self.publish()
        self.assertIsNone(self.services.release)
        self.assert_no_go_tag()

    def test_conflicting_remote_tag_is_not_replaced(self):
        self.services.references['v' + VERSION] = {'type': 'commit', 'sha': '2' * 40}
        with self.assertRaisesRegex(module.ReleaseError,
                                    'does not match release commit'):
            self.publish()
        self.assertFalse(self.services.events)

    def test_existing_annotated_tag_is_peeled(self):
        self.services.references[VERSION] = {'type': 'tag', 'sha': '3' * 40}
        self.services.tag_objects['3' * 40] = {'type': 'commit', 'sha': HEAD}
        self.publish()
        self.assertIn('go-tag', self.services.events)

    def test_dirty_checkout_stops_before_services(self):
        self.services.dirty = True
        with self.assertRaisesRegex(module.ReleaseError, 'Commit reviewed'):
            self.publish()
        self.assertFalse(self.services.events)

    def test_missing_github_commit_explains_prerequisite_before_publication(self):
        self.services.commit_error = (
            f'gh failed: gh: No commit found for SHA: {HEAD} (HTTP 422)')
        with self.assertRaises(module.ReleaseError) as raised:
            self.publish()
        message = str(raised.exception)
        self.assertIn(f'Local release commit {HEAD[:7]} is not on GitHub', message)
        self.assertIn('rerun make release v=0.1.0', message)
        self.assertIn('No artifacts or tags were published', message)
        self.assertFalse(self.services.events)
        self.assertIsNone(self.services.release)
        self.assert_no_go_tag()

    def test_github_authentication_failure_is_not_reported_as_missing_commit(self):
        self.services.commit_error = 'gh: Bad credentials (HTTP 401)'
        with self.assertRaisesRegex(module.ReleaseError, 'Bad credentials') as raised:
            self.publish()
        self.assertNotIn('is not on GitHub', str(raised.exception))
        self.assertFalse(self.services.events)

    def test_checkout_changes_during_validation_stop_before_publication(self):
        self.services.edit_during_download = True
        with self.assertRaisesRegex(module.ReleaseError, 'checkout changed'):
            self.publish()
        self.assert_no_go_tag()
        self.assertIsNone(self.services.release)

    def test_wrong_draft_commit_is_not_reused(self):
        self.services.release = {'tag_name': VERSION, 'draft': True,
                                 'target_commitish': '2' * 40}
        with self.assertRaisesRegex(module.ReleaseError, 'different release commit'):
            self.publish()
        self.assert_no_go_tag()

    def test_existing_draft_branch_resolves_to_reviewed_commit(self):
        self.services.release = {'tag_name': VERSION, 'draft': True,
                                 'target_commitish': 'main'}
        self.publish()
        self.assertIn('go-tag', self.services.events)

    def test_github_credentials_use_yamlstar_secret_file(self):
        del self.release.environment['GH_TOKEN']
        secrets = self.directory / 'secrets.yaml'
        secrets.write_text('github:\n  token: fixture-private-token\n')
        self.release.environment['YS_SECRETS_FILE'] = str(secrets)
        completed = subprocess.CompletedProcess([], 0, 'fixture-private-token\n')
        with patch.object(module.subprocess, 'run', return_value=completed) as call:
            self.release.credentials()
        self.assertEqual(self.release.environment['GH_TOKEN'], 'fixture-private-token')
        self.assertIn('github.token', call.call_args.args[0][2])
        self.assertEqual(call.call_args.kwargs['stderr'], subprocess.DEVNULL)

    def test_git_reference_request_uses_json_and_private_environment(self):
        release = module.Release(VERSION)
        payload = {'ref': 'refs/tags/v' + VERSION, 'sha': HEAD}
        completed = subprocess.CompletedProcess([], 0, json.dumps(payload))
        with patch.object(module.subprocess, 'run', return_value=completed) as call:
            self.assertEqual(release.api('git/refs', payload), payload)
        self.assertEqual(json.loads(call.call_args.kwargs['input']), payload)
        arguments = call.call_args.args[0]
        self.assertEqual(arguments[-4:], ('--method', 'POST', '--input', '-'))
        self.assertNotIn('fixture-private-token', str(arguments))
        self.assertEqual(call.call_args.kwargs['env']['GH_TOKEN'],
                         'fixture-private-token')

    def test_central_configuration_waits_for_automatic_publication(self):
        pom = ET.parse(ROOT / 'pom.xml')
        ns = {'m': 'http://maven.apache.org/POM/4.0.0'}
        configuration = pom.find(
            './/m:plugin[m:artifactId="central-publishing-maven-plugin"]/'
            'm:configuration', ns)
        self.assertEqual(configuration.find('m:autoPublish', ns).text, 'true')
        self.assertEqual(configuration.find('m:waitUntil', ns).text, 'published')


if __name__ == '__main__':
    unittest.main()
