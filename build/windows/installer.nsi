; Offline, per-user Windows AMD64 installer. Built by package.py.
Unicode true
ManifestDPIAware true
RequestExecutionLevel user
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "WinVer.nsh"
!include "x64.nsh"
!include "WordFunc.nsh"
!include "FileFunc.nsh"

!define APP_KEY "Software\TerminalAddict\SADPTools"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\SADPTools"
!define RUNTIME_KEY "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
Name "SADPTools"
OutFile "${OUTPUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\SADPTools"
; Runtime is already compressed; non-solid compression allows it to be skipped.
SetCompressor lzma
ShowInstDetails show
ShowUninstDetails show
VIProductVersion "${FILE_VERSION}"
VIAddVersionKey "ProductName" "SADPTools Setup"
VIAddVersionKey "FileDescription" "SADPTools offline installer"
VIAddVersionKey "FileVersion" "${FILE_VERSION}"
VIAddVersionKey "ProductVersion" "${PRODUCT_VERSION}"
VIAddVersionKey "LegalCopyright" "Copyright (c) 2026 SADPTools contributors"
!define MUI_ICON "${PROJECT_ROOT}/build/windows/icon.ico"
!define MUI_UNICON "${PROJECT_ROOT}/build/windows/icon.ico"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_RIGHT
!define MUI_HEADERIMAGE_BITMAP "${PROJECT_ROOT}/build/windows/installer-header.bmp"
!define MUI_HEADERIMAGE_UNBITMAP "${PROJECT_ROOT}/build/windows/installer-header.bmp"
!define MUI_WELCOMEFINISHPAGE_BITMAP "${PROJECT_ROOT}/build/windows/installer-sidebar.bmp"
!define MUI_UNWELCOMEFINISHPAGE_BITMAP "${PROJECT_ROOT}/build/windows/installer-sidebar.bmp"
!define MUI_WELCOMEPAGE_TEXT "Setup will install SADPTools and any required display components.$\r$\n$\r$\nEverything is included. No separate downloads or internet connection are needed.$\r$\n$\r$\nClose SADPTools before continuing."
!insertmacro MUI_PAGE_WELCOME
!define MUI_LICENSEPAGE_CHECKBOX
!insertmacro MUI_PAGE_LICENSE "${SETUP_TERMS}"
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\SADPTools.exe"
!define MUI_FINISHPAGE_RUN_NOTCHECKED
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH
!insertmacro MUI_LANGUAGE "English"

Var RuntimeReady
Var RuntimeExit

Function .onInit
    ${IfNot} ${AtLeastWin10}
        MessageBox MB_OK|MB_ICONSTOP "SADPTools requires Windows 10 or Windows 11 (64-bit Intel/AMD)." /SD IDOK
        SetErrorLevel 64
        Quit
    ${EndIf}
    ${IfNot} ${IsNativeAMD64}
        MessageBox MB_OK|MB_ICONSTOP "This installer is for 64-bit Intel/AMD Windows PCs." /SD IDOK
        SetErrorLevel 65
        Quit
    ${EndIf}
    SetShellVarContext current
    SetRegView 64
    ReadRegStr $0 HKCU "${APP_KEY}" "InstallDir"
    ${If} $0 != ""
        StrCpy $INSTDIR $0
    ${EndIf}
    ; Silent deployment requires explicit agreement to the bundled runtime terms.
    ${If} ${Silent}
        ${GetParameters} $0
        ClearErrors
        ${GetOptions} $0 "/ACCEPTLICENSE=" $1
        ${If} ${Errors}
        ${OrIf} $1 != "yes"
            SetErrorLevel 66
            Quit
        ${EndIf}
    ${EndIf}
FunctionEnd

; Microsoft documents these two registry locations. 0.0.0.0 is not installed.
; Check HKLM in the 32-bit view (WOW6432Node), then HKCU in the 64-bit view.
Function DetectRuntime
    StrCpy $RuntimeReady 0
    SetRegView 32
    ReadRegStr $0 HKLM "${RUNTIME_KEY}" "pv"
    ${If} $0 != ""
        ${VersionCompare} $0 "0.0.0.0" $1
        ${If} $1 == 1
            StrCpy $RuntimeReady 1
        ${EndIf}
    ${EndIf}
    SetRegView 64
    ReadRegStr $0 HKCU "${RUNTIME_KEY}" "pv"
    ${If} $0 != ""
        ${VersionCompare} $0 "0.0.0.0" $1
        ${If} $1 == 1
            StrCpy $RuntimeReady 1
        ${EndIf}
    ${EndIf}
FunctionEnd

Section "SADPTools" SEC_APP
    Call DetectRuntime
    ${If} $RuntimeReady != 1
        DetailPrint "Preparing the application display component. This may take a few minutes..."
        InitPluginsDir
        SetOutPath "$PLUGINSDIR"
        ; Preserve the exact Microsoft-signed redistributable; no network bootstrapper.
        SetCompress off
        File /oname=DisplayRuntimeSetup.exe "${WEBVIEW2_INSTALLER}"
        SetCompress auto
        ClearErrors
        ExecWait '"$PLUGINSDIR\DisplayRuntimeSetup.exe" /silent /install' $RuntimeExit
        ${If} ${Errors}
            StrCpy $RuntimeExit "could not start"
        ${EndIf}
        ${If} $RuntimeExit == 3010
            SetRebootFlag true
        ${EndIf}
        DetailPrint "Display component installer result: $RuntimeExit"
        ; Verify registration rather than treating an exit code as proof of success.
        Call DetectRuntime
        Delete "$PLUGINSDIR\DisplayRuntimeSetup.exe"
        ${If} $RuntimeReady != 1
            MessageBox MB_OK|MB_ICONSTOP "Setup could not prepare the application display component (result: $RuntimeExit). Close other installers and try again. If this PC is managed, your IT administrator may need to allow the installation." /SD IDOK
            SetErrorLevel 67
            Abort "The required display component was not installed. SADPTools has not been updated."
        ${EndIf}
    ${Else}
        DetailPrint "The application display component is already available."
    ${EndIf}

    SetOutPath "$INSTDIR"
    SetOverwrite on
    ClearErrors
    File /oname=SADPTools.exe "${APP_BINARY}"
    ${If} ${Errors}
        SetErrorLevel 69
        Abort "Could not install SADPTools. Close the application and run setup again."
    ${EndIf}
    File "${PROJECT_ROOT}/LICENSE"
    File "${PROJECT_ROOT}/THIRD_PARTY_NOTICES.md"
    SetOutPath "$INSTDIR\LICENSES"
    File "${PROJECT_ROOT}/LICENSES/*.txt"
    ${If} ${Errors}
        SetErrorLevel 69
        Abort "Could not install the application notices. Check available disk space and run setup again."
    ${EndIf}
    SetOutPath "$INSTDIR"
    ClearErrors
    WriteUninstaller "$INSTDIR\Uninstall.exe"
    ${If} ${Errors}
        SetErrorLevel 69
        Abort "Could not create the uninstaller. Check available disk space and run setup again."
    ${EndIf}
    CreateDirectory "$SMPROGRAMS\SADPTools"
    CreateShortcut "$SMPROGRAMS\SADPTools\SADPTools.lnk" "$INSTDIR\SADPTools.exe"
    CreateShortcut "$SMPROGRAMS\SADPTools\Uninstall SADPTools.lnk" "$INSTDIR\Uninstall.exe"
    WriteRegStr HKCU "${APP_KEY}" "InstallDir" "$INSTDIR"
    WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "SADPTools"
    WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${PRODUCT_VERSION}"
    WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "TerminalAddict"
    WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\SADPTools.exe"
    WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
    WriteRegStr HKCU "${UNINSTALL_KEY}" "QuietUninstallString" '$\"$INSTDIR\Uninstall.exe$\" /S'
    WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
    WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
    ${If} ${Errors}
        SetErrorLevel 69
        Abort "Could not register the installation or create shortcuts. Run setup again after checking folder permissions."
    ${EndIf}
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    WriteRegDWORD HKCU "${UNINSTALL_KEY}" "EstimatedSize" $0
SectionEnd

Function un.onInit
    SetShellVarContext current
    SetRegView 64
FunctionEnd

Section "Uninstall"
    ; Only remove files owned by us. Preserve exports, settings and shared WebView2.
    ClearErrors
    Delete "$INSTDIR\SADPTools.exe"
    ${If} ${Errors}
        MessageBox MB_OK|MB_ICONSTOP "Close SADPTools, then run the uninstaller again." /SD IDOK
        SetErrorLevel 68
        Abort "SADPTools is still running or could not be removed."
    ${EndIf}
    Delete "$INSTDIR\LICENSE"
    Delete "$INSTDIR\THIRD_PARTY_NOTICES.md"
    Delete "$INSTDIR\LICENSES\dependencies.txt"
    Delete "$INSTDIR\LICENSES\hikvision-tooling-MIT.txt"
    Delete "$INSTDIR\LICENSES\Microsoft-WebView2.txt"
    Delete "$INSTDIR\LICENSES\NSIS.txt"
    RMDir "$INSTDIR\LICENSES"
    Delete "$INSTDIR\Uninstall.exe"
    Delete "$SMPROGRAMS\SADPTools\SADPTools.lnk"
    Delete "$SMPROGRAMS\SADPTools\Uninstall SADPTools.lnk"
    RMDir "$SMPROGRAMS\SADPTools"
    DeleteRegKey HKCU "${APP_KEY}"
    DeleteRegKey HKCU "${UNINSTALL_KEY}"
    RMDir "$INSTDIR"
SectionEnd
