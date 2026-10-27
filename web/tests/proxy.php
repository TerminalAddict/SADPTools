<?php
declare(strict_types=1);
require dirname(__DIR__) . '/public/_app/ReleaseProxy.php';
$count = 0;
function check(bool $yes, string $message): void { global $count; if (!$yes) throw new RuntimeException($message); $count++; }
function rejects(Closure $fn, string $message): void { try { $fn(); } catch (Throwable) { check(true, $message); return; } throw new RuntimeException($message); }
function fixture(string $version, bool $draft = false): array {
    $assets = []; $checksums = '';
    foreach (ReleaseProxy::TARGETS as $key => [$pattern, $type]) {
        $name = sprintf($pattern, $version); $hash = hash('sha256', 'synthetic download');
        $checksums .= "$hash  $name\n";
        $assets[] = ['name' => $name, 'size' => strlen('synthetic download'), 'state' => 'uploaded', 'digest' => 'sha256:' . $hash, 'browser_download_url' => "https://github.com/TerminalAddict/SADPTools/releases/download/v$version/$name"];
    }
    $assets[] = ['name'=>'SHA256SUMS', 'size'=>strlen($checksums), 'state'=>'uploaded', 'browser_download_url'=>"https://github.com/TerminalAddict/SADPTools/releases/download/v$version/SHA256SUMS"];
    return [['tag_name'=>"v$version", 'draft'=>$draft, 'published_at'=>'2026-10-27T00:00:00Z','assets'=>$assets],$checksums];
}
[$beta,$hashes] = fixture('0.6.0-beta.1'); [$stable] = fixture('0.6.0'); [$future] = fixture('0.7.0-beta.1'); [$draft] = fixture('9.0.0',true);
[$rc] = fixture('0.6.0-rc.1'); [$rc2] = fixture('0.6.0-rc.2'); [$rc10] = fixture('0.6.0-rc.10');
check(ReleaseProxy::selectRelease([$rc,$beta])['version']==='0.6.0-rc.1','RC outranks beta');
check(ReleaseProxy::selectRelease([$rc,$stable])['version']==='0.6.0','Stable outranks RC');
check(ReleaseProxy::selectRelease([$rc2,$rc10])['version']==='0.6.0-rc.10','RC sequence ordered numerically');
check(ReleaseProxy::selectRelease([$rc])['assets']['windows-setup.exe']['name']==='SADPTools-0.6.0-rc.1-Setup-windows-amd64.exe','RC installer filename selected');
check(ReleaseProxy::selectRelease([$beta,$stable,$draft])['version']==='0.6.0','Stable outranks beta; draft excluded');
check(ReleaseProxy::selectRelease([$stable,$future])['version']==='0.7.0-beta.1','Newer beta selected');
check(ReleaseProxy::selectRelease([$future,$stable])['version']==='0.7.0-beta.1','Order independent');
$partial=$future; array_shift($partial['assets']);
rejects(fn()=>ReleaseProxy::selectRelease([$beta,$partial]),'Incomplete newest release must not silently publish old version');
$malicious=$beta; $malicious['assets'][0]['browser_download_url']='http://127.0.0.1/secret';
rejects(fn()=>ReleaseProxy::selectRelease([$malicious]),'Reject arbitrary asset target');
$assetSet=ReleaseProxy::selectRelease([$beta])['assets'];
check(count(ReleaseProxy::verifyChecksums($assetSet,$hashes))===7,'All seven hashes matched');
check($assetSet['macos.dmg']['name']==='SADPTools-0.6.0-beta.1-macos-universal.dmg','Universal Mac disk image selected');
check($assetSet['macos.zip']['type']==='application/zip','Mac archive has ZIP content type');
$missingMac=$future;
$missingMac['assets']=array_values(array_filter($missingMac['assets'],fn($a)=>!str_ends_with($a['name'],'-macos-universal.dmg')));
rejects(fn()=>ReleaseProxy::selectRelease([$stable,$missingMac]),'Missing newest Mac image must not silently fall back');
rejects(fn()=>ReleaseProxy::verifyChecksums($assetSet,$hashes.$hashes),'Duplicate hash rejected');
rejects(fn()=>ReleaseProxy::verifyChecksums($assetSet,"garbage\n"),'Malformed manifest rejected');
$bad=$assetSet; $bad['linux.deb']['digest']='sha256:'.str_repeat('0',64);
rejects(fn()=>ReleaseProxy::verifyChecksums($bad,$hashes),'Conflicting API hash rejected');
foreach (['http://github.com/x','https://github.com.evil.test/x','https://127.0.0.1/','https://github.com/another/project','file:///etc/passwd','https://user@release-assets.githubusercontent.com/x','https://release-assets.githubusercontent.com:8080/x'] as $url) check(!ReleaseProxy::approvedURL($url),'Reject '.$url);
check(ReleaseProxy::approvedURL('https://release-assets.githubusercontent.com/asset?sig=example'),'Allow signed CDN URL');
check(ReleaseProxy::range('bytes=0-3',10)===[0,3],'Explicit range');
check(ReleaseProxy::range('bytes=7-',10)===[7,9],'Open range');
check(ReleaseProxy::range('bytes=-3',10)===[7,9],'Suffix range');
check(ReleaseProxy::range('bytes=1-99',10)===[1,9],'Clamped range');
foreach (['bytes=10-20','bytes=7-2','bytes=-0','bytes=0-1,3-4','bytes=-','not-range'] as $range) rejects(fn()=>ReleaseProxy::range($range,10),'Invalid range rejected');
$dir=sys_get_temp_dir().'/sadptools-proxy-test-'.bin2hex(random_bytes(8)); mkdir($dir,0700);
try {
 $calls=0; $networkDown=false; $badBinary=false; $release=$beta; $manifest=$hashes;
 $fetch=function(string $url,int $limit,?string $destination) use (&$calls,&$networkDown,&$badBinary,&$release,&$manifest): string {
  $calls++; if($networkDown) throw new RuntimeException('Synthetic outage');
  if(str_contains($url,'api.github.com/')) return json_encode([$release],JSON_THROW_ON_ERROR);
  if(str_ends_with($url,'SHA256SUMS')) return $manifest;
  if($destination) {file_put_contents($destination,$badBinary?'bad bytes':'synthetic download'); return '';}
  throw new RuntimeException('Unexpected test request');
 };
 $proxy=new ReleaseProxy($dir,$fetch); $r=$proxy->release();
 check($r['release']['version']==='0.6.0-beta.1','Initial cache');
 check($calls===2,'One catalogue and checksum fetch');
 $public=json_encode($proxy->publicMetadata($r),JSON_THROW_ON_ERROR);
 check(!preg_match('/github|upstream|browser_download_url/i',$public),'Public metadata contains no upstream links');
 $proxy->release(); check($calls===2,'Fresh metadata cache reused');
 $legacy=$r; unset($legacy['release']['assets']['macos.dmg'],$legacy['release']['assets']['macos.zip']);
 file_put_contents($dir.'/release.json',json_encode($legacy));
 $migrated=$proxy->release();
 check($calls===4 && count($migrated['release']['assets'])===7,'Fresh legacy cache refreshed immediately for Mac downloads');
 $mac=$proxy->publicMetadata($migrated)['assets']['macos.dmg'];
 check($mac['download']==='/download/macos.dmg','Mac download stays first-party');
 check($migrated['release']['assets']['macos.dmg']['type']==='application/x-apple-diskimage','DMG content type preserved through cache');
 $macPath=$proxy->assetFile($migrated['release']['assets']['macos.zip']);
 check(hash_file('sha256',$macPath)===$migrated['release']['assets']['macos.zip']['sha256'],'Mac archive checksum verified');
 $asset=$r['release']['assets']['windows-setup.exe'];
 $path=$proxy->assetFile($asset); check(file_get_contents($path)==='synthetic download','Verified bytes cached');
 $before=$calls; $proxy->assetFile($asset); check($before===$calls,'Cached file reused');
 file_put_contents($path,'tampered'); $badBinary=true;
 rejects(fn()=>$proxy->assetFile($asset),'Corrupt or short transfer never served');
 check(!is_file($path)&&count(glob($dir.'/download-*'))===0,'Failed transfer cleaned');
 $badBinary=false;
 $r['fetchedAt']=time()-600; file_put_contents($dir.'/release.json',json_encode($r));
 $networkDown=true; $stale=$proxy->release(); check($proxy->publicMetadata($stale)['stale'],'Outage uses labelled cached metadata');
 $before=$calls; $proxy->release(); check($calls===$before,'Outage backoff avoids request flood');
 $r['fetchedAt']=time()-90000; file_put_contents($dir.'/release.json',json_encode($r));
 rejects(fn()=>$proxy->release(),'Very old cache must fail');
 unlink($dir.'/refresh-failed'); $networkDown=false; [$release,$manifest]=fixture('0.6.0-rc.1');
 $next=$proxy->release(); check($next['release']['version']==='0.6.0-rc.1','RC picked up automatically after beta cache expires');
 check($proxy->publicMetadata($next)['assets']['linux.deb']['download']==='/download/linux.deb','Public URL unchanged after release');
} finally { foreach(glob($dir.'/*') as $p) unlink($p); rmdir($dir); }
echo "$count proxy assertions passed\n";
