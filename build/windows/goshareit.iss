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
