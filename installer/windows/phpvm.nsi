; phpvm NSIS installer (buildable on Linux with makensis)
; Build example:
;   makensis -DAPP_VERSION=0.1.1-alpha.2 -DINPUT_EXE=dist\\phpvm.exe -DOUT_DIR=dist installer\\windows\\phpvm.nsi

!ifndef APP_VERSION
  !define APP_VERSION "0.1.1-alpha"
!endif
!ifndef INPUT_EXE
  !define INPUT_EXE "..\\..\\dist\\phpvm.exe"
!endif
!ifndef OUT_DIR
  !define OUT_DIR "..\\..\\dist"
!endif

!include "LogicLib.nsh"
!include "MUI2.nsh"

!define APP_NAME "phpvm"
!define APP_PUBLISHER "Lauro Vitor"
!define APP_URL "https://github.com/laurovitor/phpvm"
!define APP_UNINST_KEY "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\phpvm"

Name "${APP_NAME} ${APP_VERSION}"
OutFile "${OUT_DIR}\\phpvm-setup.exe"
InstallDir "$LOCALAPPDATA\phpvm"
RequestExecutionLevel user
Unicode true
Icon "..\\..\\assets\\icons\\icon_3.ico"
UninstallIcon "..\\..\\assets\\icons\\icon_3.ico"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "PortugueseBR"
!insertmacro MUI_LANGUAGE "Spanish"

LangString DESC_INSTALLED ${LANG_ENGLISH} "Installed to $INSTDIR"
LangString DESC_INSTALLED ${LANG_PORTUGUESEBR} "Instalado em $INSTDIR"
LangString DESC_INSTALLED ${LANG_SPANISH} "Instalado en $INSTDIR"
LangString DESC_RESTART ${LANG_ENGLISH} "Restart terminal to use phpvm command."
LangString DESC_RESTART ${LANG_PORTUGUESEBR} "Reinicie o terminal para usar o comando phpvm."
LangString DESC_RESTART ${LANG_SPANISH} "Reinicia la terminal para usar el comando phpvm."

Function .onInit
  !insertmacro MUI_LANGDLL_DISPLAY
FunctionEnd

Section "Install"
  SetOutPath "$INSTDIR\\bin"
  File "${INPUT_EXE}"

  ; Add to user PATH (idempotent)
  ReadRegStr $0 HKCU "Environment" "Path"
  StrCpy $1 "$INSTDIR\\bin"
  StrLen $2 $0
  ${If} $2 == 0
    StrCpy $0 "$1"
  ${Else}
    ; only append if not already present
    Push $0
    Push $1
    Call StrStr
    Pop $3
    ${If} $3 == ""
      StrCpy $0 "$0;$1"
    ${EndIf}
  ${EndIf}
  WriteRegExpandStr HKCU "Environment" "Path" "$0"

  ; Uninstall registration
  WriteRegStr HKCU "${APP_UNINST_KEY}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "${APP_UNINST_KEY}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "${APP_UNINST_KEY}" "Publisher" "${APP_PUBLISHER}"
  WriteRegStr HKCU "${APP_UNINST_KEY}" "URLInfoAbout" "${APP_URL}"
  WriteRegStr HKCU "${APP_UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${APP_UNINST_KEY}" "UninstallString" '"$INSTDIR\\Uninstall.exe"'
  WriteRegDWORD HKCU "${APP_UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${APP_UNINST_KEY}" "NoRepair" 1

  WriteUninstaller "$INSTDIR\\Uninstall.exe"

  DetailPrint "$(DESC_INSTALLED)"
  DetailPrint "$(DESC_RESTART)"
SectionEnd

Section "Uninstall"
  ; Remove from user PATH
  ReadRegStr $0 HKCU "Environment" "Path"
  StrCpy $1 "$INSTDIR\\bin"
  Push $0
  Push ";$1"
  Call un.StrReplace
  Pop $0
  Push $0
  Push "$1;"
  Call un.StrReplace
  Pop $0
  ${If} $0 == "$1"
    StrCpy $0 ""
  ${EndIf}
  WriteRegExpandStr HKCU "Environment" "Path" "$0"

  Delete "$INSTDIR\\bin\\phpvm.exe"
  RMDir "$INSTDIR\\bin"
  Delete "$INSTDIR\\Uninstall.exe"
  RMDir "$INSTDIR"

  DeleteRegKey HKCU "${APP_UNINST_KEY}"
SectionEnd

; --- helper funcs ---
Function StrStr
  Exch $R1
  Exch
  Exch $R2
  Push $R3
  Push $R4
  Push $R5
  StrLen $R3 $R1
  StrLen $R4 $R2
  StrCpy $R5 0
loop:
  StrCpy $0 $R2 $R3 $R5
  StrCmp $0 $R1 done
  IntOp $R5 $R5 + 1
  IntCmp $R5 $R4 done loop done
done:
  StrCmp $R5 $R4 0 +2
  StrCpy $R2 ""
  StrCpy $R2 $R2 "" $R5
  Pop $R5
  Pop $R4
  Pop $R3
  Exch $R2
FunctionEnd

Function StrReplace
  Exch $R2 ; needle
  Exch
  Exch $R1 ; haystack
  Push $R3
  Push $R4
  Push $R5
  StrCpy $R3 ""
  loop2:
    Push $R1
    Push $R2
    Call StrStr
    Pop $R4
    StrCmp $R4 "" done2
    StrLen $R5 $R1
    StrLen $0 $R4
    IntOp $0 $R5 - $0
    StrCpy $R3 "$R3$R1" $0
    StrLen $0 $R2
    StrCpy $R1 $R4 "" $0
    Goto loop2
  done2:
    StrCpy $R1 "$R3$R1"
    Pop $R5
    Pop $R4
    Pop $R3
    Exch $R1
FunctionEnd

Function un.StrStr
  Exch $R1
  Exch
  Exch $R2
  Push $R3
  Push $R4
  Push $R5
  StrLen $R3 $R1
  StrLen $R4 $R2
  StrCpy $R5 0
uloop:
  StrCpy $0 $R2 $R3 $R5
  StrCmp $0 $R1 udone
  IntOp $R5 $R5 + 1
  IntCmp $R5 $R4 udone uloop udone
udone:
  StrCmp $R5 $R4 0 +2
  StrCpy $R2 ""
  StrCpy $R2 $R2 "" $R5
  Pop $R5
  Pop $R4
  Pop $R3
  Exch $R2
FunctionEnd

Function un.StrReplace
  Exch $R2
  Exch
  Exch $R1
  Push $R3
  Push $R4
  Push $R5
  StrCpy $R3 ""
  uloop2:
    Push $R1
    Push $R2
    Call un.StrStr
    Pop $R4
    StrCmp $R4 "" udone2
    StrLen $R5 $R1
    StrLen $0 $R4
    IntOp $0 $R5 - $0
    StrCpy $R3 "$R3$R1" $0
    StrLen $0 $R2
    StrCpy $R1 $R4 "" $0
    Goto uloop2
  udone2:
    StrCpy $R1 "$R3$R1"
    Pop $R5
    Pop $R4
    Pop $R3
    Exch $R1
FunctionEnd
