; ADSBiq Airport feeder — one-click Windows installer (Inno Setup 6).
;
; Assembles the feed agent + decoders into a single .exe that:
;   1. installs the WinUSB driver for the RTL-SDR dongle (silent, via libwdi),
;   2. drops the agent + bundled decoders,
;   3. registers a background Windows service (WinSW) that runs the agent,
;   4. optionally tags the device with the school / FBO name the user typed.
;
; Build inputs live under installer\dist\ — see installer\BUILD.md for how to
; assemble them from the CI artifacts. Compile with: iscc flightpatch.iss

#define AppName "Flightpatch"
#define AppVer "0.4.0"
#define AppPublisher "ADSBiq"
#define AppURL "https://flightpatch.app/airport"
; RTL2832U (all RTL-SDR dongles): USB VID 0x0BDA, PID 0x2838
#define RtlVid "0x0BDA"
#define RtlPid "0x2838"

[Setup]
AppName={#AppName}
AppVersion={#AppVer}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
DefaultDirName={autopf}\ADSBiq
DefaultGroupName=ADSBiq
DisableProgramGroupPage=yes
OutputDir=out
OutputBaseFilename=flightpatch-setup-{#AppVer}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
; Flightpatch splash art (164x314 + 328x628 hi-DPI); Inno picks by monitor DPI.
WizardImageFile=assets\wizard-164x314.bmp,assets\wizard-328x628.bmp
WizardImageStretch=no
; driver install + service registration require elevation
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
ArchitecturesAllowed=x64compatible
UninstallDisplayName={#AppName}

[Messages]
; Onboarding copy — plain language, no jargon. (See PRODUCT_UX.md for the full spec.)
WelcomeLabel2=In about a minute, your airfield goes live at flightpatch.app/airport — every takeoff, landing, and pattern, in real time.%n%nWe'll set up your dongle and start feeding automatically. No Raspberry Pi, no config, no hassle.
FinishedHeadingLabel=You're live! 🎉
FinishedLabelNoIcons=Your airfield is now feeding the ADSBiq network. Aircraft may take a minute to appear on the map.
FinishedLabel=Your airfield is now feeding the ADSBiq network. Aircraft may take a minute to appear on the map.

[Files]
; the agent
Source: "dist\adsbiq-feed-agent.exe"; DestDir: "{app}"; Flags: ignoreversion
; decoders + their runtime DLLs (dumpvdl2 today; dump1090 added when built)
Source: "dist\decoders\*"; DestDir: "{app}\decoders"; Flags: ignoreversion recursesubdirs createallsubdirs
; WinSW service host (renamed) — reads adsbiq-service.xml written at install time
Source: "dist\service\WinSW.exe"; DestDir: "{app}\service"; DestName: "adsbiq-service.exe"; Flags: ignoreversion
; silent WinUSB driver installer (libwdi wdi-simple)
Source: "dist\driver\wdi-simple.exe"; DestDir: "{app}\driver"; Flags: ignoreversion skipifsourcedoesntexist

[Icons]
Name: "{group}\ADSBiq live map"; Filename: "{#AppURL}"
Name: "{group}\Uninstall ADSBiq Feeder"; Filename: "{uninstallexe}"

[Run]
; 1) bind WinUSB to the dongle silently (--silent = no Zadig UI). Self-contained exe.
Filename: "{app}\driver\wdi-simple.exe"; \
  Parameters: "--vid {#RtlVid} --pid {#RtlPid} --type 0 --silent --name ""RTL2832U"""; \
  StatusMsg: "Installing USB driver for your dongle..."; \
  Flags: runhidden waituntilterminated skipifdoesntexist
; 2) install + start the background service (config written in CurStepChanged)
Filename: "{app}\service\adsbiq-service.exe"; Parameters: "install"; Flags: runhidden waituntilterminated
Filename: "{app}\service\adsbiq-service.exe"; Parameters: "start"; Flags: runhidden waituntilterminated
; 3) offer to open the live map
Filename: "{#AppURL}"; Description: "Open my live airfield map"; Flags: postinstall shellexec nowait

[UninstallRun]
Filename: "{app}\service\adsbiq-service.exe"; Parameters: "stop"; Flags: runhidden waituntilterminated; RunOnceId: "StopSvc"
Filename: "{app}\service\adsbiq-service.exe"; Parameters: "uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "DelSvc"

[Code]
var
  OrgPage: TInputQueryWizardPage;

procedure InitializeWizard;
begin
  // Make the dongle timing impossible to miss — the driver only binds to a
  // dongle that's connected during setup.
  CreateOutputMsgPage(wpWelcome,
    'Plug in your dongle',
    'Connect your RTL-SDR receiver now',
    'Before you continue, plug your RTL-SDR dongle into a USB port on this computer.' + #13#10 + #13#10 +
    'Setup installs the dongle''s driver, so it needs to be connected right now — otherwise it won''t be able to feed.' + #13#10 + #13#10 +
    'No dongle yet? You can close Setup and simply run it again once your dongle is plugged in.');

  OrgPage := CreateInputQueryPage(wpSelectDir,
    'Name your airfield (optional)',
    'Put your flight school or FBO on the map.',
    'If you enter a name, your feeder shows up on the ADSBiq network under it. ' +
    'Leave it blank to feed anonymously. You can change this later.');
  OrgPage.Add('School / FBO / organization name:', False);
end;

// XML-escape the few characters that matter for the WinSW config.
function XmlEscape(const S: string): string;
begin
  Result := S;
  StringChangeEx(Result, '&', '&amp;', True);
  StringChangeEx(Result, '<', '&lt;', True);
  StringChangeEx(Result, '>', '&gt;', True);
  StringChangeEx(Result, '"', '&quot;', True);
end;

// Write the WinSW service definition (points at the agent + bundled decoders,
// injects the optional org name) before the service is installed.
procedure WriteServiceConfig;
var
  Xml, Args, Org, Path: string;
begin
  Org := Trim(OrgPage.Values[0]);
  Args := '--decoders "' + ExpandConstant('{app}\decoders') + '"';
  if Org <> '' then
    Args := Args + ' --org "' + Org + '"';

  Xml :=
    '<service>' + #13#10 +
    '  <id>adsbiq-agent</id>' + #13#10 +
    '  <name>ADSBiq Feeder Agent</name>' + #13#10 +
    '  <description>Feeds ADS-B / VDL2 from your dongle to the ADSBiq network.</description>' + #13#10 +
    '  <executable>' + XmlEscape(ExpandConstant('{app}\adsbiq-feed-agent.exe')) + '</executable>' + #13#10 +
    '  <arguments>' + XmlEscape(Args) + '</arguments>' + #13#10 +
    '  <onfailure action="restart" delay="10 sec"/>' + #13#10 +
    '  <resetfailure>1 hour</resetfailure>' + #13#10 +
    '  <priority>idle</priority>' + #13#10 +
    '  <startmode>Automatic</startmode>' + #13#10 +
    '  <log mode="roll-by-size"><sizeThreshold>5120</sizeThreshold><keepFiles>3</keepFiles></log>' + #13#10 +
    '</service>' + #13#10;

  Path := ExpandConstant('{app}\service\adsbiq-service.xml');
  if not SaveStringToFile(Path, Xml, False) then
    MsgBox('Could not write the service configuration.', mbError, MB_OK);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    WriteServiceConfig;  // runs before the [Run] service-install entries
end;
