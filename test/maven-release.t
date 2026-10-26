#!/usr/bin/env perl
use strict;
use warnings;
use Test::More;
use File::Path qw(make_path);
use File::Spec;
use File::Temp qw(tempdir);
use IPC::Open3;
use JSON::PP qw(decode_json);
use MIME::Base64 qw(encode_base64);
use Symbol qw(gensym);

make_path('.cache');
my $temporary = tempdir('release-test-XXXXXX', DIR => '.cache', CLEANUP => 1);
my $secrets = File::Spec->rel2abs("$temporary/secrets.yaml");
my $capture = File::Spec->rel2abs("$temporary/maven.json");
my $maven = File::Spec->rel2abs("$temporary/fake maven");
my $ys = $ENV{YS} // 'ys';
my $token = encode_base64('release-user:p&ss:word', '');
my $fallback = encode_base64('fallback-user:fallback-password', '');

open my $fake, '>', $maven or die "$maven: $!";
print {$fake} <<'PERL';
#!/usr/bin/env perl
use strict;
use warnings;
use JSON::PP qw(encode_json);
my @args = @ARGV;
my ($settings);
for my $i (0 .. $#args - 1) {
    $settings = $args[$i + 1] if $args[$i] eq '--settings';
}
open my $input, '<', $settings or die $!;
local $/;
my $xml = <$input>;
close $input;
open my $out, '>', $ENV{MAVEN_CAPTURE} or die $!;
print {$out} encode_json({args => \@args, settings => $xml,
    path => $settings, mode => (stat($settings))[2] & 0777,
    username => $ENV{YAMLSTAR_CENTRAL_USERNAME},
    password => $ENV{YAMLSTAR_CENTRAL_PASSWORD},
    key => $ENV{GPG_KEY_ID}, passphrase => $ENV{MAVEN_GPG_PASSPHRASE}});
close $out;
exit($ENV{MAVEN_EXIT} // 0);
PERL
close $fake or die $!;
chmod 0755, $maven or die $!;

sub fixture {
    my ($yaml) = @_;
    open my $file, '>', $secrets or die $!;
    print {$file} $yaml;
    close $file or die $!;
}

sub run_release {
    my ($mode, $overrides) = @_;
    local %ENV = %ENV;
    delete @ENV{qw(CENTRAL_TOKEN GPG_KEY_ID GPG_PASSPHRASE
        MAVEN_GPG_PASSPHRASE YAMLSTAR_CENTRAL_USERNAME
        YAMLSTAR_CENTRAL_PASSWORD)};
    $ENV{YS} = $ys;
    $ENV{YS_SECRETS_FILE} = $secrets;
    $ENV{MAVEN_CAPTURE} = $capture;
    $ENV{MAVEN_EXIT} = 0;
    @ENV{keys %$overrides} = values %$overrides if $overrides;
    unlink $capture if -e $capture;
    my $error = gensym;
    my $pid = open3(my $input, my $output, $error, $^X,
        'util/maven-release', $mode, $maven, '/fixture/repository');
    close $input;
    local $/;
    my $log = (<$output> // '') . (<$error> // '');
    waitpid($pid, 0);
    my $status = $? >> 8;
    my $data;
    if (-f $capture) {
        open my $file, '<', $capture or die $!;
        $data = decode_json(<$file>);
        close $file or die $!;
    }
    return ($status, $data, $log);
}

fixture("java:\n  token: $token\ncentral:\n  token: $fallback\n" .
    "gpg:\n  key-id: fixture-signing-key\n  passphrase: 'signing password'\n");
my ($status, $data, $log) = run_release('release');
is($status, 0, 'release reads the YAMLStar secrets format');
is($data->{username}, 'release-user', 'java.token takes precedence');
is($data->{password}, 'p&ss:word', 'token password retains special characters');
is($data->{key}, 'fixture-signing-key', 'signing key comes from gpg.key-id');
is($data->{passphrase}, 'signing password', 'passphrase uses Maven environment');
is($data->{mode}, 0600, 'temporary settings have private permissions');
ok(!-e $data->{path}, 'temporary settings are removed after success');
like($data->{settings}, qr/\$\{env.YAMLSTAR_CENTRAL_PASSWORD\}/,
    'Maven settings refer to environment variables');
for my $value ($token, 'release-user', 'p&ss:word', 'signing password') {
    unlike($log, qr/\Q$value\E/, 'credential value is absent from output');
    unlike($data->{settings}, qr/\Q$value\E/,
        'credential value is absent from temporary settings');
}
ok(grep($_ eq '-Dcentral.skipUpload=false', @{$data->{args}}),
    'release enables upload');
ok(grep($_ eq '-Dmaven.repo.local=/fixture/repository', @{$data->{args}}),
    'managed Maven repository is retained');

fixture("central:\n  token: $fallback\ngpg:\n  key-id: fallback-key\n");
($status, $data, $log) = run_release('release');
is($status, 0, 'central.token fallback works with real YAMLScript');
is($data->{username}, 'fallback-user', 'fallback token is decoded');

fixture("gpg:\n  key-id: bundle-key\n");
($status, $data, $log) = run_release('bundle');
is($status, 0, 'offline bundle needs no Central token');
ok(grep($_ eq '-Dcentral.skipUpload=true', @{$data->{args}}),
    'bundle disables uploading');

($status, $data, $log) = run_release('release');
isnt($status, 0, 'release rejects a missing publishing token');
ok(!defined $data, 'missing token never launches Maven');

($status, $data, $log) = run_release('release', {
    CENTRAL_TOKEN => $token, GPG_KEY_ID => 'environment-key',
    GPG_PASSPHRASE => 'environment-password'});
is($status, 0, 'existing environment credentials work');
is($data->{key}, 'environment-key', 'environment signing key takes precedence');
is($data->{passphrase}, 'environment-password', 'GPG_PASSPHRASE is supported');

fixture("java:\n  token: invalid-token\ngpg:\n  key-id: fixture-key\n");
($status, $data, $log) = run_release('release');
isnt($status, 0, 'invalid encoded token is rejected');
unlike($log, qr/invalid-token/, 'invalid token is not echoed');
ok(!defined $data, 'invalid token never launches Maven');

fixture("java: [MALFORMED-PRIVATE-VALUE\n");
($status, $data, $log) = run_release('release');
isnt($status, 0, 'malformed secret file is rejected');
unlike($log, qr/MALFORMED-PRIVATE-VALUE/, 'parser diagnostics do not expose secrets');

fixture("gpg:\n  key-id: fixture-key\n");
($status, $data, $log) = run_release('bundle', {MAVEN_EXIT => 42});
is($status, 42, 'Maven failure status propagates');
ok(!-e $data->{path}, 'temporary settings are removed after Maven failure');

open my $pom, '<', 'pom.xml' or die $!;
my $source = do { local $/; <$pom> };
close $pom or die $!;
my ($version) = $source =~ m{<version>([^<]+)</version>};
for my $case ([$version, 0], ['', 1], ['999.999.999', 1]) {
    my $error = gensym;
    my $pid = open3(my $input, my $output, $error, $^X,
        'util/maven-release', 'check-version', $case->[0]);
    close $input;
    my $log = do { local $/; (<$output> // '') . (<$error> // '') };
    waitpid($pid, 0);
    is(($? >> 8) != 0 ? 1 : 0, $case->[1],
        length($case->[0]) ? "version check: $case->[0]" : 'v is required');
}

done_testing;
