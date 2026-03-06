; phpvm Windows Installer (Inno Setup)
; Build with: iscc installer\windows\phpvm.iss

#define MyAppName "phpvm"
#define MyAppPublisher "Lauro Vitor"
#define MyAppURL "https://github.com/laurovitor/phpvm"
#define MyAppExeName "phpvm.exe"
#define MyAppVersion "0.1.1-alpha"

[Setup]
AppId={{D4B8B6A1-7D53-4B9A-92C7-2D2D41E8A7C1}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
DefaultDirName={localappdata}\phpvm
DefaultGroupName=phpvm
OutputDir=..\..\dist
OutputBaseFilename=phpvm-setup
Compression=lzma
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=lowest
DisableProgramGroupPage=yes
ArchitecturesAllowed=x64
ArchitecturesInstallIn64BitMode=x64
UninstallDisplayIcon={app}\bin\phpvm.exe
ChangesEnvironment=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "addtopath"; Description: "Add phpvm to PATH (current user)"; GroupDescription: "Additional tasks:"; Flags: checkedonce

[Files]
Source: "..\..\dist\phpvm.exe"; DestDir: "{app}\bin"; Flags: ignoreversion

[Dirs]
Name: "{userprofile}\.phpvm"
Name: "{userprofile}\.phpvm\versions"

[Icons]
Name: "{autoprograms}\phpvm"; Filename: "{app}\bin\phpvm.exe"

[Code]
procedure CurStepChanged(CurStep: TSetupStep);
var
  ExistingPath: string;
begin
  if CurStep = ssPostInstall then
  begin
    if WizardIsTaskSelected('addtopath') then
    begin
      if not RegQueryStringValue(HKCU, 'Environment', 'Path', ExistingPath) then
        ExistingPath := '';

      if Pos(LowerCase(ExpandConstant('{app}\bin')), LowerCase(ExistingPath)) = 0 then
      begin
        if (Length(ExistingPath) > 0) and (ExistingPath[Length(ExistingPath)] <> ';') then
          ExistingPath := ExistingPath + ';';
        ExistingPath := ExistingPath + ExpandConstant('{app}\bin');
        RegWriteStringValue(HKCU, 'Environment', 'Path', ExistingPath);
      end;
    end;
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  ExistingPath: string;
  BinPath: string;
begin
  if CurUninstallStep = usUninstall then
  begin
    BinPath := LowerCase(ExpandConstant('{app}\bin'));
    if RegQueryStringValue(HKCU, 'Environment', 'Path', ExistingPath) then
    begin
      StringChangeEx(ExistingPath, ';' + ExpandConstant('{app}\bin'), '', True);
      StringChangeEx(ExistingPath, ExpandConstant('{app}\bin') + ';', '', True);
      if LowerCase(Trim(ExistingPath)) = BinPath then
        ExistingPath := '';
      RegWriteStringValue(HKCU, 'Environment', 'Path', ExistingPath);
    end;
  end;
end;
