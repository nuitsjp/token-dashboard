Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "x64.nsh"
!include "app.nsh"

Name "${APP_NAME}"
OutFile "..\..\..\bin\${INSTALLER_NAME}"
InstallDir "$LOCALAPPDATA\Programs\${APP_ID}"
InstallDirRegKey HKCU "Software\${APP_ID}" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma
VIProductVersion "${APP_VERSION}.0"
VIAddVersionKey "ProductName" "${APP_NAME}"
VIAddVersionKey "FileDescription" "${APP_NAME} Setup"
VIAddVersionKey "FileVersion" "${APP_VERSION}"
VIAddVersionKey "ProductVersion" "${APP_VERSION}"
VIAddVersionKey "LegalCopyright" "MIT"
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Japanese"

Var UpdatePID
Var Restart
Var UpdateMutex

Function .onInit
  SetShellVarContext current
  SetRegView 64
  ReadRegStr $0 HKCU "Software\${APP_ID}" "InstallDir"
  ${If} $0 != ""
    StrCpy $INSTDIR $0
  ${EndIf}
  !if "${APP_ARCH}" == "arm64"
    ${IfNot} ${IsNativeARM64}
      MessageBox MB_ICONSTOP "このインストーラーはWindows ARM64専用です。"
      Abort
    ${EndIf}
  !else
    ${IfNot} ${IsNativeAMD64}
      MessageBox MB_ICONSTOP "このインストーラーはWindows x64専用です。"
      Abort
    ${EndIf}
  !endif
  ; Prevent competing installers without changing Wails' single-instance mechanism.
  System::Call 'kernel32::CreateMutexW(p0, i0, w "Local\${APP_ID}.installer") p .r0 ?e'
  Pop $1
  StrCpy $UpdateMutex $0
  ${If} $0 == 0
    MessageBox MB_ICONSTOP "インストーラーの排他制御を開始できません。"
    Abort
  ${EndIf}
  ${If} $1 == 183
    MessageBox MB_ICONSTOP "別のインストーラーが実行中です。"
    Abort
  ${EndIf}
  ${GetParameters} $0
  ${GetOptions} $0 "/UPDATEPID=" $UpdatePID
  ${GetOptions} $0 "/RESTART=" $Restart
FunctionEnd

Function WaitForOldProcess
  ${If} $UpdatePID != ""
    ; Access SYNCHRONIZE only. Never terminate the caller forcibly.
    System::Call 'kernel32::OpenProcess(i 0x00100000, i0, i $UpdatePID) p .r0'
    ${If} $0 != 0
      System::Call 'kernel32::WaitForSingleObject(p r0, i60000) i .r1'
      System::Call 'kernel32::CloseHandle(p r0)'
      ${If} $1 != 0
        MessageBox MB_ICONSTOP "アプリの終了を確認できませんでした。更新していません。"
        SetErrorLevel 10
        Abort
      ${EndIf}
    ${EndIf}
  ${EndIf}
  ; Also refuse a manually started installer while the executable is locked.
  IfFileExists "$INSTDIR\${APP_EXE}" 0 done
  ClearErrors
  FileOpen $0 "$INSTDIR\${APP_EXE}" a
  IfErrors locked
  FileClose $0
  Goto done
locked:
  MessageBox MB_ICONSTOP "アプリを終了してから、インストーラーを再実行してください。"
  SetErrorLevel 11
  Abort
done:
FunctionEnd

Section "Application"
  Call WaitForOldProcess
  ; WebView2 Evergreen is a prerequisite. We do not silently run a network
  ; installer on offline machines; the README links Microsoft's installer.
  SetRegView 32
  ReadRegStr $0 HKLM "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 == ""
    ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${EndIf}
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    MessageBox MB_ICONSTOP "Microsoft Edge WebView2 Runtimeを先にインストールしてください。"
    SetErrorLevel 12
    Abort
  ${EndIf}
  SetRegView 64
  SetOutPath "$INSTDIR"
  ClearErrors
  File "..\..\..\bin\${APP_EXE}"
  IfErrors failed
  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKCU "Software\${APP_ID}" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "DisplayIcon" "$INSTDIR\${APP_EXE}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "UninstallString" '$\"$INSTDIR\uninstall.exe$\"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "NoRepair" 1
  CreateShortcut "$SMPROGRAMS\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"
  IfErrors failed
  Goto done
failed:
  MessageBox MB_ICONSTOP "インストールに失敗しました。同じインストーラーを手動で再実行してください。利用者データは削除していません。"
  SetErrorLevel 13
  Abort
done:
SectionEnd

Function .onInstSuccess
  ${If} $Restart == "1"
    Exec '"$INSTDIR\${APP_EXE}"'
    ${If} ${Errors}
      MessageBox MB_ICONEXCLAMATION "更新は完了しましたが再起動できません。スタートメニューから起動してください。"
    ${EndIf}
  ${EndIf}
FunctionEnd
Function .onGUIEnd
  ${If} $UpdateMutex != 0
    System::Call 'kernel32::CloseHandle(p $UpdateMutex)'
  ${EndIf}
FunctionEnd
Section "Uninstall"
  SetShellVarContext current
  SetRegView 64
  Delete "$INSTDIR\${APP_EXE}"
  IfErrors 0 +3
    MessageBox MB_ICONSTOP "アプリを終了してからアンインストールしてください。"
    Abort
  Delete "$SMPROGRAMS\${APP_NAME}.lnk"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  DeleteRegKey HKCU "Software\${APP_ID}"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}"
  ; Never remove the separate AppData directory.
SectionEnd
