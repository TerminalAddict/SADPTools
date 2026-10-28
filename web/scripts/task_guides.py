"""Task guides checked against the desktop controls and v1.0.1 documentation.

These describe user-operated workflows; publishing them never contacts cameras.
"""

TASK_GUIDES = [
    {
        'file': 'activate-hikvision-camera.html',
        'title': 'How to Activate a Hikvision Camera with SADPTools',
        'description': 'Activate a supported inactive Hikvision camera with SADPTools on Windows, Linux or Mac. Identify the device, set its admin password and verify the result.',
        'heading': 'How to activate a Hikvision camera',
        'intro': 'For a compatible inactive Hikvision device, SADPTools can set the initial administrator password over the local network. Discover the device, confirm its identity, activate it once, then verify the result.',
        'image': 'activation',
        'caption': 'The activation form in SADPTools. Linux screenshot with a synthetic inactive device; no password was entered and no activation was sent.',
        'related': ['sadp-tool.html', 'change-camera-ip-address.html', 'camera-setup-linux-mac.html'],
        'body': '''
<h2>Before you begin</h2>
<p>Install <a href="/#downloads">SADPTools for Windows, Linux or Apple Mac</a>. Connect the computer to the camera's local network and confirm that the camera has power. Use equipment you own or are authorised to administer. Have the camera's label or your installation record available so that you can check its MAC address and serial number.</p>
<p>This workflow is for an <strong>inactive</strong> device whose firmware supports the implemented SADP activation method. It does not deactivate an existing camera or change the password of an already activated device. The same desktop controls are available on all three platforms; camera firmware determines compatibility.</p>
<h2>Activate the selected camera</h2>
<ol>
<li><strong>Discover the device.</strong> In Live discovery, choose the adapter connected to the camera network, or scan all suitable adapters. Wait for the inventory to show the camera and its activation status.</li>
<li><strong>Confirm its identity.</strong> Select the camera and compare its MAC address and serial number with the physical device. An IP address alone is not enough to distinguish equipment that may have duplicate factory addresses.</li>
<li><strong>Finish or stop the scan.</strong> Stop an active discovery operation before starting a device write.</li>
<li><strong>Open Activate device.</strong> Review the identity at the top of the form. SADPTools automatically chooses an adapter that discovered this device. If several adapters saw it, Connection options provides an optional override; the request still uses one adapter.</li>
<li><strong>Set and confirm the new admin password.</strong> Use a unique password and retain it in your approved password store. The app checks its basic password rules; the camera can impose additional requirements.</li>
<li><strong>Select Activate this device once.</strong> Read the result before taking another action. SADPTools does not automatically retry activation.</li>
<li><strong>Choose Close and rediscover.</strong> Check the same device by MAC and serial. Once its address is reachable, test a login through its detected web interface using the new credentials.</li>
</ol>
<h2>Understand the result</h2>
<p>An acknowledged result means a matching success reply was received. Independent rediscovery and a successful login provide stronger confirmation that the camera is ready. A rejection means the camera explicitly refused the operation; review the reported reason and its firmware requirements.</p>
<p>An <strong>unconfirmed</strong> result or timeout does not prove that activation failed. The camera may have changed state while its reply was lost. Rediscover and check before trying again. Stop waiting cancels waiting for a response; it cannot undo a request already delivered.</p>
<h2>If the camera is missing or already active</h2>
<p>Check the adapter, power, VLAN, firewall and discovery log. SADP is a local discovery protocol; a route to the camera's web page does not guarantee a SADP activation path. See the <a href="/cctv-camera-discovery.html">discovery troubleshooting guide</a>.</p>
<p>For an already active camera, use the existing administrator credentials. If they are unknown, the supported Password recovery workflow may offer previously configured security questions or a support-provided code. Recovery availability depends on firmware and does not bypass camera security.</p>
<h2>After activation</h2>
<p>Use <a href="/change-camera-ip-address.html">Configure IP / DHCP</a> if the camera needs a different network address. An out-of-subnet device can be visible to local SADP discovery while its web interface is still unreachable. Save the camera into a site after checking its identity and current settings.</p>''',
    },
    {
        'file': 'change-camera-ip-address.html',
        'title': 'Change a Hikvision Camera IP Address or DHCP | SADPTools',
        'description': 'Change the IP address, subnet mask, gateway or DHCP setting of a compatible Hikvision camera. Review the complete network form and verify the new address.',
        'heading': 'How to change a camera IP address or enable DHCP',
        'intro': 'SADPTools can update the network settings of a compatible activated Hikvision device found through local SADP discovery. You need its current administrator password and the correct network settings for the site.',
        'image': 'network',
        'caption': 'Configure network in SADPTools. Linux screenshot with synthetic addresses; no network settings were applied.',
        'related': ['activate-hikvision-camera.html', 'find-camera-different-subnet.html', 'transfer-camera-site.html'],
        'body': '''
<h2>Plan the address before changing the camera</h2>
<p>Record the existing IPv4 address, subnet mask, gateway, DHCP state and SDK command port. Agree the new settings with whoever manages the network. Check address reservations, the DHCP pool and known assignments. A failed ping or a silent scan is not proof that an address is free.</p>
<p>Changing an address can interrupt an NVR, video management server, Live View session or other client. Plan how those clients will reach the camera afterwards. An inactive device first needs <a href="/activate-hikvision-camera.html">activation</a>.</p>
<h2>Use the network-configuration form</h2>
<ol>
<li><strong>Discover and select the camera.</strong> Confirm the MAC address and serial number, then finish or stop discovery before starting a write.</li>
<li><strong>Choose Configure IP / DHCP</strong> in device details. The Configure network dialog uses values from the camera's discovery reply. Missing fields stay empty; do not assume an empty field is a valid default.</li>
<li><strong>Review the complete configuration.</strong> Enter the intended IPv4 address, subnet mask and gateway. Confirm the SDK command port separately from the camera's HTTP, HTTPS or RTSP ports.</li>
<li><strong>Select static addressing or DHCP.</strong> If DHCP was not advertised, make an explicit choice. When enabling DHCP, the form still sends the stored static address settings, so those fields must also be valid.</li>
<li><strong>Enter the current admin password.</strong> Check the camera identity and settings again. Normally the discovered adapter is chosen automatically; Connection options allows an override if more than one adapter saw the device.</li>
<li><strong>Select Apply network changes once.</strong> Read the acknowledgement, rejection or unconfirmed result. There are no automatic retries.</li>
<li><strong>Choose Close and rediscover.</strong> Locate the camera by MAC and serial, check its newly advertised configuration, and open the detected web interface once routing permits it.</li>
</ol>
<h2>What each field means</h2>
<dl><dt>IPv4 address</dt><dd>The intended static address, or the stored static value sent with a DHCP request. Choose an address appropriate for the camera's network.</dd><dt>Subnet mask</dt><dd>Defines the local IPv4 subnet. Use the site's actual mask.</dd><dt>Gateway</dt><dd>The router used for destinations outside that subnet. The form supports <code>0.0.0.0</code> to mean no gateway when that is the intended configuration.</dd><dt>SDK command port</dt><dd>The device's SDK service port. This is not its web-interface port. Preserve the advertised value unless the installation calls for a deliberate change.</dd><dt>DHCP</dt><dd>Allows a DHCP server to supply the camera's address where supported. Rediscover or consult the lease table to find the assigned address.</dd></dl>
<h2>If the new address is not reachable</h2>
<p>A successful SADP reply and browser reachability are separate checks. Confirm that your computer can route to the new subnet and that the network permits the camera's services. Check the camera's mask and gateway and any recorder configuration affected by the change.</p>
<p>If the operation timed out, inspect the device's current state before resending. Cancelling the wait cannot reverse a change already made. Use the <a href="/find-camera-different-subnet.html">different-subnet guide</a> when the route works but local discovery does not.</p>
<h2>Update the saved installation</h2>
<p>Keep the verified address with the saved site, and update clients that reference the old IP. Review any imported stream addresses that still refer to the old address. <a href="/transfer-camera-site.html">Export the site</a> when the updated inventory needs to travel to another laptop.</p>''',
    },
    {
        'file': 'find-camera-different-subnet.html',
        'title': 'Find or Add a Camera on a Different Subnet | SADPTools',
        'description': 'Reach a camera across a router or VPN with SADPTools. Add a known IP, inspect an authorised IPv4 range, or use a remote discovery host on the camera network.',
        'heading': 'How to find a camera on a different subnet',
        'intro': 'If your computer has a working route to a camera, SADPTools can check its IP directly even when the camera does not share your subnet. Local SADP multicast discovery is a separate connection path.',
        'image': 'add-ip',
        'caption': 'Add camera by IP in SADPTools. Demonstration setup screen, before any connection to a camera.',
        'related': ['cctv-camera-discovery.html', 'change-camera-ip-address.html', 'transfer-camera-site.html'],
        'body': '''
<h2>If you already know the camera's IP</h2>
<ol>
<li><strong>Confirm the route.</strong> Connect to the required LAN or VPN. If you can open the camera in a browser, note the address, HTTP/HTTPS protocol and any custom port. A browser using a proxy may have a different connection path from a direct camera client.</li>
<li><strong>Open Add camera by IP</strong> from Sites or the IP scanner. Choose the site that should receive the device when the dialog asks for a destination.</li>
<li><strong>Enter the camera's IPv4 address.</strong> Open Custom ports and identification if the camera needs a nonstandard port. Read-only sign-in lets you select a saved credential or supply a one-time login.</li>
<li><strong>Select Check camera.</strong> Read the returned identity and service information. If an HTTPS certificate is untrusted, compare its fingerprint with the camera before explicitly trusting the connection.</li>
<li><strong>Review the result and choose Save camera.</strong> Confirm that the returned device is the intended camera. Open its detected web interface or select Open Live View after saving where the required services and credentials are available.</li>
</ol>
<p>Reachable web access does not imply reachable RTSP or ONVIF. A firewall may allow one service and block another. Use the actual camera ports and inspect the specific failing service before changing device settings.</p>
<p>A one-time login is cleared after checking and is not saved as a camera password. Assign a saved credential if you want the site to retain it. If identity cannot be verified, the app requires explicit confirmation before saving the address as unverified.</p>
<h2>If you know the network but not the address</h2>
<p>Use <strong>IP scanner</strong> for a network you are authorised to inspect. Choose the appropriate adapter, enter a CIDR or start/end range and select <strong>Calculate scope</strong>. Review Probe settings, then select <strong>Start scan</strong> and approve the exact range where requested. Public, routed or large scopes require explicit confirmation.</p>
<p>Review responding addresses and their service evidence. An address's details offer <strong>Add camera by IP</strong> so that you can identify equipment and save it to a site. Stop / Cancel retains results already collected. No response is inconclusive; it does not establish that the address is unassigned or that no camera exists there.</p>
<h2>If you need SADP discovery on the remote network</h2>
<p>Use a trusted SSH discovery host connected to that network. It needs the compatible <code>sadptools</code> CLI and the correct local network access. Verify the SSH host-key fingerprint, then run discovery through that host and review the returned inventory.</p>
<p>The remote host performs local discovery in the camera's broadcast domain. A routed laptop cannot obtain that behaviour simply by selecting a local adapter with a different IP. Remote observations retain their origin and do not automatically enable local SADP configuration writes.</p>
<h2>Why a browser works while SADP finds nothing</h2>
<p>HTTP and HTTPS normally use routed unicast connections. SADP discovery uses multicast on UDP port 37020 and normally remains on the local network. Correct IP routing can therefore provide browser access without forwarding discovery traffic.</p>
<p>A different case is an out-of-subnet camera on the <em>same</em> local Layer 2 network. SADP may discover that device even though its web page is not yet reachable. The <a href="/sadp-tool.html">SADP guide</a> explains that distinction.</p>
<h2>What to verify after adding the camera</h2>
<p>Check the identity, detected web address, authentication and the service used for the next task. Saved inventory is not proof of current connectivity. Security-question recovery can use a routed web connection on supported firmware; activation and SADP network changes still need their appropriate discovery and write path.</p>''',
    },
    {
        'file': 'camera-setup-linux-mac.html',
        'title': 'Configure CCTV Cameras on Linux or Mac | SADPTools',
        'description': 'Install SADPTools on Linux or Apple Mac and discover compatible cameras. Learn the desktop workflow, network permissions, saved credentials and platform limits.',
        'heading': 'How to configure cameras on Linux or Apple Mac',
        'intro': 'SADPTools provides a native desktop interface on Linux and Apple Mac for camera discovery, supported Hikvision activation and network changes, Live View and saved sites. Windows uses the same workflows.',
        'image': 'discovery',
        'caption': 'The shared discovery workspace, captured on Linux with demonstration devices. This image is not evidence of Mac hardware testing.',
        'related': ['activate-hikvision-camera.html', 'change-camera-ip-address.html', 'transfer-camera-site.html'],
        'body': '''
<h2>Install on Linux</h2>
<p>Download the <a href="/#downloads">Linux AMD64 package</a> for your distribution. DEB packages suit supported Ubuntu/Debian versions; RPM packages suit supported Fedora releases. A manual tar.gz package includes <code>install.sh</code> for a per-user installation. These are x86-64 packages, not Linux ARM builds.</p>
<p>In the directory containing the downloaded file, install the DEB with <code>sudo apt install ./your-downloaded-file.deb</code>, or the RPM with <code>sudo dnf install ./your-downloaded-file.rpm</code>. Substitute the actual filename. The package manager resolves declared dependencies. For the manual package, extract it, enter its folder and run <code>./install.sh</code>; the script checks dependencies before installing.</p>
<p>The desktop needs GTK4 and WebKitGTK 6.0 on a supported distribution. Older distributions may lack the required versions. Open <strong>SADPTools</strong> from the GNOME, KDE or other desktop application menu as your normal user. The desktop requires a graphical session; the separate <code>sadptools</code> CLI can run in a terminal or over SSH.</p>
<h2>Install on Apple Mac</h2>
<p>Download the universal Mac DMG or ZIP for Apple Silicon and Intel, targeting macOS 12 or later. Open the DMG and drag SADPTools to Applications, or extract the ZIP and move SADPTools.app there. Launch it from Applications.</p>
<p>Current Mac packages are not Developer ID signed or notarised. Read the <a href="/mac-help.html">Mac installation and Gatekeeper guide</a> before deciding whether to open them. Allow local network access when macOS asks. Mac runtime, camera and codec behaviour still require testing on physical Mac hardware; a successful build is not proof of every workflow on every Mac.</p>
<h2>Discover, select and configure</h2>
<ol>
<li><strong>Connect to the camera network.</strong> Choose the relevant adapter in Live discovery, or scan all suitable adapters.</li>
<li><strong>Start discovery and inspect the results.</strong> Match MAC addresses and serial numbers against the equipment being installed. Use the diagnostic log when a scan is empty.</li>
<li><strong>Stop the scan before a write.</strong> Choose Activate device for a compatible inactive camera, or Configure IP / DHCP for a compatible activated device with known admin credentials.</li>
<li><strong>Review and apply one action.</strong> Follow the <a href="/activate-hikvision-camera.html">activation</a> or <a href="/change-camera-ip-address.html">network configuration</a> guide. An unconfirmed result requires investigation before another write.</li>
<li><strong>Rediscover and verify.</strong> Open the detected web interface when reachable, then save the camera to a site. Use direct-IP access for cameras reachable through a router or VPN.</li>
</ol>
<h2>Live View and saved passwords</h2>
<p>Live View opens in a separate window and starts automatically after you choose it. Try a supported H.264 stream first; codec support and the camera's stream settings can affect playback, especially H.265/HEVC on Mac. Camera and account capabilities determine the available image, focus and PTZ controls.</p>
<p>Linux and Mac protect saved credentials with a local passphrase vault. Mac Keychain automatic unlock is not implemented. Windows uses its existing account protection. For portability, use <a href="/transfer-camera-site.html">Export site and Import site</a> rather than copying account-specific credential files.</p>
<h2>Update without losing site data</h2>
<p>Make an encrypted site backup, close the app and its Live View windows, then install the current Linux package or replace the Mac app in Applications. Mac updates are manual after the update check. Site data and preferences live outside the app bundle.</p>''',
    },
    {
        'file': 'transfer-camera-site.html',
        'title': 'Transfer Camera Sites and Passwords Between Laptops | SADPTools',
        'description': 'Export and import a complete encrypted camera site, including known passwords and NX Witness information. Move between Windows, Linux and Mac laptops.',
        'heading': 'How to transfer a camera site and its saved passwords',
        'intro': 'Use Export site to create an encrypted .sadpsite file, then Import site on the destination laptop. The file carries the saved site and known credentials independently of each laptop’s local password protection.',
        'image': 'backup',
        'caption': 'The site-backup screen in SADPTools. Demonstration screenshot with no real site data or passphrase shown; current controls are labelled Export site and Import site.',
        'related': ['cctv-installer-software.html', 'camera-setup-linux-mac.html', 'find-camera-different-subnet.html'],
        'body': '''
<h2>Check what is saved before exporting</h2>
<p>The transfer includes camera records, saved usernames and passwords, default/shared credentials and overrides, notes, tags, address reservations, switches and their credentials, history, saved scans and reports, plans, documents and attachments. Known imported stream addresses and NX Witness information travel with the site when present.</p>
<p>Unknown passwords remain unknown. Credentials used for a one-session login but never saved to the site are not part of a site export. Check the site's Credentials settings before relying on the file as a complete handover.</p>
<h2>On the source laptop</h2>
<ol>
<li><strong>Open Sites and select the installation.</strong> Confirm that the equipment and supporting records belong to the site you intend to transfer.</li>
<li><strong>Choose Export site.</strong> Export report produces an inventory report without passwords; use Export site for the complete portable file.</li>
<li><strong>Unlock saved credentials if prompted.</strong> Windows can use its account protection. A passphrase-based vault requires its local saved-password passphrase.</li>
<li><strong>Set a site file password of at least 12 characters.</strong> This protects the exported file and is separate from camera passwords and each laptop's vault passphrase.</li>
<li><strong>Save the .sadpsite file.</strong> Transfer it through your chosen approved method, and provide the file password separately to the authorised recipient.</li>
</ol>
<h2>On the destination laptop</h2>
<ol>
<li><strong>Install a current compatible SADPTools version.</strong> Use v1.0.1 or newer for sites containing NX Witness server, user, storage and licence information.</li>
<li><strong>Choose Sites → Import site.</strong> Enter the site file password, choose the .sadpsite file and review the import summary.</li>
<li><strong>Check the proposed name and counts.</strong> Review camera, credential, attachment and NX inventory counts where supplied.</li>
<li><strong>Select Import as new site.</strong> Create or unlock the destination's local saved-password protection if requested. The importer re-encrypts secrets for this laptop.</li>
<li><strong>Review the imported site.</strong> Open Edit site and choose the destination laptop's local network adapter before rescanning. Adapter bindings from the source laptop are cleared; saved network ranges remain.</li>
</ol>
<p>Import creates a separate site and preserves existing sites, even if they share a name. It does not connect to cameras, reset passwords or write network settings. Confirm the transferred inventory on the destination before relying on it as your only copy.</p>
<h2>Importing an NX Camera Info JSON export</h2>
<p>Export the required cameras as JSON from NX Camera Info. If known camera passwords should be included, reveal them before exporting. In SADPTools, choose Import site, leave the site file password blank for this JSON format, select the file and review the summary.</p>
<p>A full report can also contain NX Witness system, servers, users, storage and licences. SADPTools stores this information encrypted and exposes a read-only <strong>NX Witness</strong> button only on sites containing that metadata. Older camera-only imports cannot recover data that was absent or discarded; import the newer full JSON as a new site.</p>
<p>An NX JSON export with revealed passwords is plaintext. Keep the source file private. For later moves, use the encrypted .sadpsite export. Ordinary CSV, JSON or HTML inventory reports are not complete site-transfer files, and importing an inventory does not establish that its cameras are currently reachable.</p>''',
    },
]
