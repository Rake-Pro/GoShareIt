; GoShareIt per-user installer. Built by release.yml:
;   iscc /DAppVersion=<ver> /DBinDir=<abs path to built exes> /O<outdir> goshareit.iss
; Per-user install (no admin): {autopf} + PrivilegesRequired=lowest resolves to
; %LOCALAPPDATA%\Programs, which also lets the in-app updater swap the exes
; without elevation.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef BinDir
  #define BinDir "..\..\dist\windows"
#endif

[Setup]
AppId={{B7E8A0D2-4C1F-4E7B-9D3A-6F2C8A5E1B47}
AppName=GoShareIt
AppVersion={#AppVersion}
AppPublisher=Rake-Pro
DefaultDirName={autopf}\GoShareIt
PrivilegesRequired=lowest
DisableProgramGroupPage=yes
OutputBaseFilename=GoShareIt_{#AppVersion}_windows_amd64_setup
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\goshareit.exe
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes

[Tasks]
Name: "startup"; Description: "Start GoShareIt when you log in"; Flags: unchecked

[Files]
Source: "{#BinDir}\goshareit.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#BinDir}\goshareit-editor.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#BinDir}\goshareit-settings.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\GoShareIt"; Filename: "{app}\goshareit.exe"
Name: "{userstartup}\GoShareIt"; Filename: "{app}\goshareit.exe"; Tasks: startup

[Run]
Filename: "{app}\goshareit.exe"; Description: "Launch GoShareIt"; Flags: nowait postinstall skipifsilent

[Code]
// Smart App Control (Windows 11 clean installs) blocks unsigned binaries with
// no per-app exception. This installer is not code-signed, so when SAC is in
// evaluation mode (or, rarely, On and this build slipped through) tell the
// user up front: turn it off, or take the Microsoft Store build instead. In
// the plain blocked case this installer never runs; the README repeats the
// steps.
function InitializeSetup(): Boolean;
var
  State: Cardinal;
  Msg: String;
  Answer, ErrorCode: Integer;
begin
  Result := True;
  if not RegQueryDWordValue(HKLM, 'SYSTEM\CurrentControlSet\Control\CI\Policy',
      'VerifiedAndReputablePolicyState', State) then
    Exit;
  if State = 0 then
    Exit;
  if State = 2 then
    Msg := 'Smart App Control is in evaluation mode on this PC. Once Windows switches it on, it will block this copy of GoShareIt and its updates.'
  else
    Msg := 'Smart App Control is turned on on this PC. It will block this copy of GoShareIt and its updates.';
  Msg := Msg + ' GoShareIt is free, open-source software and this download is not ' +
    'code-signed; Smart App Control has no per-app exception.' + #13#10#13#10 +
    'You have two options:' + #13#10 +
    '  1. Turn Smart App Control off: Windows Security > App & browser control > ' +
    'Smart App Control settings > Off, then continue this setup.' + #13#10 +
    '  2. Cancel this setup and install GoShareIt from the Microsoft Store instead. ' +
    'The Store build is signed by Microsoft and runs with Smart App Control on.' +
    #13#10#13#10 +
    'Yes = open Windows Security and continue setup' + #13#10 +
    'No = open the Microsoft Store and cancel setup' + #13#10 +
    'Cancel = continue setup anyway';
  Answer := MsgBox(Msg, mbConfirmation, MB_YESNOCANCEL);
  if Answer = IDYES then
    ShellExec('open', 'windowsdefender://appbrowser', '', '', SW_SHOWNORMAL, ewNoWait, ErrorCode)
  else if Answer = IDNO then
  begin
    ShellExec('open', 'ms-windows-store://search/?query=GoShareIt', '', '', SW_SHOWNORMAL, ewNoWait, ErrorCode);
    Result := False;
  end;
end;
