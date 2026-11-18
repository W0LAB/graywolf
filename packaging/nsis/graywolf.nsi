; Graywolf NSIS installer
;
; Required defines (pass via /D on the makensis command line):
;   APP_VERSION         display version, e.g. 0.10.10
;   APP_VERSION_NUMERIC 4-part numeric version for VIProductVersion, e.g. 0.10.10.0
;   BINARY_PATH         absolute path to staged graywolf.exe
;   MODEM_PATH          absolute path to staged graywolf-modem.exe
;
; Optional:
;   OUTFILE             output installer filename
;   DESKTOP_PATH        absolute path to staged graywolf-desktop.exe (the
;                       native desktop app). When set, the main Start Menu
;                       shortcut opens the desktop app and a second
;                       shortcut keeps the browser-based mode.
;   WEBVIEW2_BOOTSTRAPPER  absolute path to MicrosoftEdgeWebview2Setup.exe;
;                       embedded and run when the WebView2 runtime the
;                       desktop app needs is missing (rare on Windows 10/11).

!ifndef APP_VERSION
  !error "APP_VERSION is required (pass /DAPP_VERSION=x.y.z)"
!endif
!ifndef APP_VERSION_NUMERIC
  !error "APP_VERSION_NUMERIC is required (pass /DAPP_VERSION_NUMERIC=x.y.z.0)"
!endif
!ifndef BINARY_PATH
  !error "BINARY_PATH is required"
!endif
!ifndef MODEM_PATH
  !error "MODEM_PATH is required"
!endif
!ifndef OUTFILE
  !define OUTFILE "graywolf_${APP_VERSION}_Windows_x86_64.exe"
!endif

!define APP_NAME      "Graywolf"
!define APP_PUBLISHER "chrissnell"
!define APP_EXE       "graywolf.exe"
!define MODEM_EXE     "graywolf-modem.exe"
!define DESKTOP_EXE   "graywolf-desktop.exe"
; Evergreen WebView2 runtime client GUID (Microsoft-documented detection key).
!define WEBVIEW2_GUID "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
!define APP_REGKEY    "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}"

Name "${APP_NAME} ${APP_VERSION}"
OutFile "${OUTFILE}"
Unicode true
InstallDir "$PROGRAMFILES64\${APP_NAME}"
InstallDirRegKey HKLM "Software\${APP_NAME}" "InstallDir"
RequestExecutionLevel admin
SetCompressor /SOLID lzma

VIProductVersion "${APP_VERSION_NUMERIC}"
VIAddVersionKey  "ProductName"     "${APP_NAME}"
VIAddVersionKey  "CompanyName"     "${APP_PUBLISHER}"
VIAddVersionKey  "FileDescription" "${APP_NAME} APRS gateway"
VIAddVersionKey  "FileVersion"     "${APP_VERSION}"
VIAddVersionKey  "ProductVersion"  "${APP_VERSION}"
VIAddVersionKey  "LegalCopyright"  "${APP_PUBLISHER}"

!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"

!define MUI_ABORTWARNING

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\..\LICENSE"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Start ${APP_NAME} now"
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchAppShortcut
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

; ---------------------------------------------------------------------------
; Helpers
; ---------------------------------------------------------------------------

; Launches the Start Menu shortcut so the app starts with the proper args
; rather than re-implementing the same command line in two places.
Function LaunchAppShortcut
  ExecShell "" "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
FunctionEnd

; The desktop app hides to the tray on close, so a plain WM_CLOSE does not
; stop it; force-stop it (and its modem child, /T) so its files can be
; replaced or removed. The SQLite databases are crash-safe.
!macro StopDesktopApp
  nsExec::ExecToStack 'taskkill /F /T /IM ${DESKTOP_EXE}'
  Pop $0
  Pop $1
  Sleep 500
!macroend

!ifdef DESKTOP_PATH
!ifdef WEBVIEW2_BOOTSTRAPPER
; Install the WebView2 runtime if it is missing. Checks the per-machine
; and per-user registrations Microsoft documents; "0.0.0.0" means removed.
Function EnsureWebView2
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\${WEBVIEW2_GUID}" "pv"
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\${WEBVIEW2_GUID}" "pv"
  ${EndIf}
  ${If} $0 != ""
  ${AndIf} $0 != "0.0.0.0"
    DetailPrint "WebView2 runtime $0 found"
    Return
  ${EndIf}
  DetailPrint "Installing the Microsoft Edge WebView2 runtime..."
  InitPluginsDir
  File "/oname=$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" "${WEBVIEW2_BOOTSTRAPPER}"
  ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install' $1
  ${If} $1 != 0
    MessageBox MB_ICONEXCLAMATION|MB_OK \
      "The Microsoft Edge WebView2 runtime could not be installed (error $1).$\n$\nThe ${APP_NAME} window needs it. You can install it from Microsoft later, or use the '${APP_NAME} (web browser)' shortcut meanwhile." \
      /SD IDOK
  ${EndIf}
FunctionEnd
!endif
!endif

; ---------------------------------------------------------------------------
; Init: 64-bit only, and silently uninstall any previous version in place
; ---------------------------------------------------------------------------

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "${APP_NAME} requires 64-bit Windows."
    Abort
  ${EndIf}
  SetRegView 64

  ReadRegStr $R0 HKLM "${APP_REGKEY}" "UninstallString"
  ReadRegStr $R1 HKLM "${APP_REGKEY}" "InstallLocation"
  StrCmp $R0 "" done

  MessageBox MB_OKCANCEL|MB_ICONQUESTION \
    "An existing ${APP_NAME} installation was found and will be removed before installing this version.$\n$\nYour data in C:\ProgramData\${APP_NAME} will be preserved.$\n$\nContinue?" \
    /SD IDOK IDOK uninst
  Abort

  uninst:
    ClearErrors
    ; _?=<dir> keeps the uninstaller running synchronously instead of
    ; respawning itself in TEMP, so we can wait for it to finish.
    ExecWait '"$R0" /S _?=$R1'
done:
FunctionEnd

Function un.onInit
  SetRegView 64
FunctionEnd

; ---------------------------------------------------------------------------
; Install
; ---------------------------------------------------------------------------

Section "Install"
  !insertmacro StopDesktopApp
  SetOutPath "$INSTDIR"
  File "/oname=${APP_EXE}"   "${BINARY_PATH}"
  File "/oname=${MODEM_EXE}" "${MODEM_PATH}"
!ifdef DESKTOP_PATH
  File "/oname=${DESKTOP_EXE}" "${DESKTOP_PATH}"
!ifdef WEBVIEW2_BOOTSTRAPPER
  Call EnsureWebView2
!endif
!endif

  ; Writable data directory under C:\ProgramData. Same path the old MSI
  ; used, so existing users keep their database on upgrade.
  SetShellVarContext all
  CreateDirectory "$APPDATA\${APP_NAME}"

  ; Grant the local Users group Modify access so non-admin shortcuts can
  ; write the database. *S-1-5-32-545 is the well-known Users SID and is
  ; locale-independent, unlike the literal "Users" group name.
  nsExec::ExecToLog 'icacls "$APPDATA\${APP_NAME}" /grant "*S-1-5-32-545:(OI)(CI)M"'
  Pop $0

  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
!ifdef DESKTOP_PATH
  ; Main shortcut opens the desktop app. With no arguments it uses the
  ; same data files as the browser-mode shortcut below
  ; (C:\ProgramData\Graywolf\graywolf.db, graywolf-history.db) and finds
  ; graywolf-modem.exe next to itself.
  CreateShortCut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" \
    "$INSTDIR\${DESKTOP_EXE}" "" "$INSTDIR\${DESKTOP_EXE}" 0
  ; Browser mode: same arguments the previous Windows service used; the
  ; UI is then at http://127.0.0.1:8080 in any browser.
  CreateShortCut "$SMPROGRAMS\${APP_NAME}\${APP_NAME} (web browser).lnk" \
    "$INSTDIR\${APP_EXE}" \
    '-config "$APPDATA\${APP_NAME}\graywolf.db" -history-db "$APPDATA\${APP_NAME}\graywolf-history.db" -modem "$INSTDIR\${MODEM_EXE}" -http 127.0.0.1:8080' \
    "$INSTDIR\${APP_EXE}" 0
!else
  ; Start Menu shortcut launches graywolf with the same arguments the
  ; previous Windows service used. The icon reuses the binary's own.
  CreateShortCut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" \
    "$INSTDIR\${APP_EXE}" \
    '-config "$APPDATA\${APP_NAME}\graywolf.db" -history-db "$APPDATA\${APP_NAME}\graywolf-history.db" -modem "$INSTDIR\${MODEM_EXE}" -http 127.0.0.1:8080' \
    "$INSTDIR\${APP_EXE}" 0
!endif
  CreateShortCut "$SMPROGRAMS\${APP_NAME}\Uninstall ${APP_NAME}.lnk" \
    "$INSTDIR\uninstall.exe"

  ; Add/Remove Programs entry
  WriteRegStr   HKLM "Software\${APP_NAME}" "InstallDir" "$INSTDIR"
  WriteRegStr   HKLM "${APP_REGKEY}" "DisplayName"          "${APP_NAME}"
  WriteRegStr   HKLM "${APP_REGKEY}" "DisplayVersion"       "${APP_VERSION}"
  WriteRegStr   HKLM "${APP_REGKEY}" "Publisher"            "${APP_PUBLISHER}"
!ifdef DESKTOP_PATH
  WriteRegStr   HKLM "${APP_REGKEY}" "DisplayIcon"          "$INSTDIR\${DESKTOP_EXE}"
!else
  WriteRegStr   HKLM "${APP_REGKEY}" "DisplayIcon"          "$INSTDIR\${APP_EXE}"
!endif
  WriteRegStr   HKLM "${APP_REGKEY}" "InstallLocation"      "$INSTDIR"
  WriteRegStr   HKLM "${APP_REGKEY}" "UninstallString"      '"$INSTDIR\uninstall.exe"'
  WriteRegStr   HKLM "${APP_REGKEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKLM "${APP_REGKEY}" "NoModify" 1
  WriteRegDWORD HKLM "${APP_REGKEY}" "NoRepair" 1

  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

; ---------------------------------------------------------------------------
; Uninstall
; ---------------------------------------------------------------------------

Section "Uninstall"
  SetShellVarContext all
  !insertmacro StopDesktopApp

  Delete "$INSTDIR\${APP_EXE}"
  Delete "$INSTDIR\${MODEM_EXE}"
  Delete "$INSTDIR\${DESKTOP_EXE}"
  Delete "$INSTDIR\uninstall.exe"
  RMDir  "$INSTDIR"

  Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME} (web browser).lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\Uninstall ${APP_NAME}.lnk"
  RMDir  "$SMPROGRAMS\${APP_NAME}"

  ; Intentionally do NOT delete $APPDATA\${APP_NAME} — preserve user data.

  DeleteRegKey HKLM "${APP_REGKEY}"
  DeleteRegKey HKLM "Software\${APP_NAME}"
SectionEnd
