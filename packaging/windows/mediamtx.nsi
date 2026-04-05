; MediaMTX NVR NSIS Installer Configuration
; Requires NSIS 3.x (https://nsis.sourceforge.io/)

!include "MUI2.nsh"
!include "nsDialogs.nsh"
!include "LogicLib.nsh"
!include "WinMessages.nsh"

; ---- General ----
!define PRODUCT_NAME "MediaMTX NVR"
!define PRODUCT_PUBLISHER "MediaMTX"
!define PRODUCT_WEB_SITE "https://github.com/bluenviron/mediamtx"
!define PRODUCT_UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}"
!define PRODUCT_UNINST_ROOT_KEY "HKLM"

!ifndef PRODUCT_VERSION
  !define PRODUCT_VERSION "0.0.0"
!endif

Name "${PRODUCT_NAME} ${PRODUCT_VERSION}"
OutFile "mediamtx-nvr-${PRODUCT_VERSION}-setup.exe"
InstallDir "$PROGRAMFILES64\MediaMTX"
InstallDirRegKey HKLM "${PRODUCT_UNINST_KEY}" "InstallLocation"
RequestExecutionLevel admin
SetCompressor /SOLID lzma

; ---- MUI Settings ----
!define MUI_ABORTWARNING
!define MUI_ICON "${NSISDIR}\Contrib\Graphics\Icons\modern-install.ico"
!define MUI_UNICON "${NSISDIR}\Contrib\Graphics\Icons\modern-uninstall.ico"

; ---- Pages ----
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\..\LICENSE"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

; ---- Install Section ----
Section "MediaMTX NVR" SecMain
  SectionIn RO

  ; Stop existing service if running
  nsExec::ExecToLog 'sc stop MediaMTX'
  Sleep 2000

  ; Install binary
  SetOutPath "$INSTDIR"
  File "..\..\tmp\mediamtx.exe"

  ; Create config directory
  CreateDirectory "$INSTDIR\config"
  IfFileExists "$INSTDIR\config\mediamtx.yml" +2 0
    File /oname=config\mediamtx.yml "..\..\mediamtx.yml"

  ; Create data directories
  CreateDirectory "$INSTDIR\data"
  CreateDirectory "$INSTDIR\data\recordings"
  CreateDirectory "$INSTDIR\data\thumbnails"
  CreateDirectory "$INSTDIR\logs"

  ; Install Windows service
  nsExec::ExecToLog 'sc create MediaMTX binPath= "\"$INSTDIR\mediamtx.exe\" \"$INSTDIR\config\mediamtx.yml\"" start= auto DisplayName= "MediaMTX NVR" obj= LocalSystem'
  nsExec::ExecToLog 'sc description MediaMTX "MediaMTX NVR - Real-time media server and network video recorder"'
  nsExec::ExecToLog 'sc failure MediaMTX reset= 86400 actions= restart/5000/restart/10000/restart/30000'

  ; Start the service
  nsExec::ExecToLog 'sc start MediaMTX'

  ; Firewall rules
  nsExec::ExecToLog 'netsh advfirewall firewall add rule name="MediaMTX RTSP" dir=in action=allow protocol=TCP localport=8554'
  nsExec::ExecToLog 'netsh advfirewall firewall add rule name="MediaMTX RTMP" dir=in action=allow protocol=TCP localport=1935'
  nsExec::ExecToLog 'netsh advfirewall firewall add rule name="MediaMTX HTTP API" dir=in action=allow protocol=TCP localport=9997'
  nsExec::ExecToLog 'netsh advfirewall firewall add rule name="MediaMTX WebRTC" dir=in action=allow protocol=UDP localport=8189'
  nsExec::ExecToLog 'netsh advfirewall firewall add rule name="MediaMTX HLS" dir=in action=allow protocol=TCP localport=8888'

  ; Create Start Menu shortcuts
  CreateDirectory "$SMPROGRAMS\${PRODUCT_NAME}"
  CreateShortCut "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall.lnk" "$INSTDIR\uninstall.exe"

  ; Write uninstaller
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Write registry keys for Add/Remove Programs
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayName" "${PRODUCT_NAME}"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "UninstallString" "$INSTDIR\uninstall.exe"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "Publisher" "${PRODUCT_PUBLISHER}"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "URLInfoAbout" "${PRODUCT_WEB_SITE}"
  WriteRegDWORD ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "NoModify" 1
  WriteRegDWORD ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "NoRepair" 1
SectionEnd

; ---- Uninstall Section ----
Section "Uninstall"
  ; Stop and remove the service
  nsExec::ExecToLog 'sc stop MediaMTX'
  Sleep 2000
  nsExec::ExecToLog 'sc delete MediaMTX'

  ; Remove firewall rules
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="MediaMTX RTSP"'
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="MediaMTX RTMP"'
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="MediaMTX HTTP API"'
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="MediaMTX WebRTC"'
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="MediaMTX HLS"'

  ; Remove files (preserve config and data)
  Delete "$INSTDIR\mediamtx.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir /r "$SMPROGRAMS\${PRODUCT_NAME}"

  ; Remove registry keys
  DeleteRegKey ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}"

  ; Remove install dir only if empty (preserves config/data)
  RMDir "$INSTDIR"

  MessageBox MB_ICONINFORMATION "Configuration and data files were preserved in $INSTDIR.$\nDelete this directory manually if no longer needed."
SectionEnd
