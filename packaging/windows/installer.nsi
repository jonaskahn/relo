; The Relo-Setup Windows installer: one relo.exe carrying the daemon and the
; tray, beside the relo.cmd shim the command line runs, with a Start Menu
; shortcut and a quiet autostart entry. The packaging script passes VERSION,
; BINARY, SHIMCMD, SHIMSH, ICON, and OUTFILE through /D defines.
;
; The install is per user: files under $LOCALAPPDATA, the uninstall entry
; and the PATH entry under HKCU, and no elevation request, so the login
; items and the shortcut always belong to the signed-in user. A previous
; all-users install is removed first, through its own uninstaller. A relo
; command from any other install refuses this one, so two products never
; fight over the same name.
;
; The install also puts the shim directory on the user PATH, so the relo
; command works from any terminal opened afterwards. An NSIS string holds
; 1024 characters and a PATH can be longer, so the value is measured before
; it is written back instead of being truncated.

!include "LogicLib.nsh"
!include "StrFunc.nsh"

; StrStr and StrRep compare case-insensitively, which is how Windows matches
; PATH entries; the Un pair is the same functions inside the uninstaller.
${Using:StrFunc} StrStr
${Using:StrFunc} StrRep
${Using:StrFunc} UnStrStr
${Using:StrFunc} UnStrRep

!ifndef VERSION
  !define VERSION "0.0.0-dev"
!endif
!ifndef BINARY
  !error "BINARY is required: pass /DBINARY=<path to relo.exe>"
!endif
!ifndef SHIMCMD
  !error "SHIMCMD is required: pass /DSHIMCMD=<path to relo.cmd>"
!endif
!ifndef SHIMSH
  !error "SHIMSH is required: pass /DSHIMSH=<path to relo>"
!endif
!ifndef ICON
  !define ICON "icon.ico"
!endif
!ifndef OUTFILE
  !define OUTFILE "Relo-Setup-${VERSION}.exe"
!endif

Name "Relo"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\Relo"
; Nothing lands in a system folder and nothing is written to HKLM, which is
; what the user level is for.
RequestExecutionLevel user
SetCompressor /SOLID lzma

Icon "${ICON}"
UninstallIcon "${ICON}"

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\Relo"
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"
!define APPROVED_KEY "Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run"
!define ENVIRONMENT_KEY "Environment"

; A PATH at or past this length cannot be read and written back through an
; NSIS string, so the installer reports the directory instead of editing.
!define PATH_LIMIT 1000

Section "Relo" SEC_MAIN
  SectionIn RO
  ; A build from before this installer went per user sits under Program
  ; Files with an administrator manifest. Its own uninstaller is the only
  ; thing that can delete it, and it elevates itself; running it silently
  ; keeps the data and removes its HKLM PATH entry, shortcut, and login
  ; items. The tray is stopped first, because the uninstaller cannot delete
  ; an executable that is still running.
  ${If} ${FileExists} "$PROGRAMFILES64\Relo\uninstall.exe"
    DetailPrint "removing the previous all-users install"
    nsExec::Exec 'taskkill /F /IM relo-desktop.exe'
    Pop $0
    ClearErrors
    ExecShellWait "" "$PROGRAMFILES64\Relo\uninstall.exe" "/S" SW_HIDE
    ${If} ${FileExists} "$PROGRAMFILES64\Relo\uninstall.exe"
      MessageBox MB_ICONEXCLAMATION|MB_OK "The previous all-users install of Relo could not be removed automatically. Remove the older Relo in Windows Settings under Apps after this install, so only one copy remains." /SD IDOK
    ${Else}
      DetailPrint "the previous all-users install is removed"
    ${EndIf}
    ; The old login values may name executables that no longer exist; the
    ; new install rewrites the desktop value, and the app rewrites the
    ; daemon value when the daemon next starts.
    Delete "$SMPROGRAMS\Relo\Relo.lnk"
    RMDir "$SMPROGRAMS\Relo"
    DeleteRegValue HKCU "${RUN_KEY}" "Relo"
    DeleteRegValue HKCU "${RUN_KEY}" "ReloDaemon"
  ${EndIf}

  Call CheckConflictingRelo
  ; An update replaces the binaries, and a running exe cannot be overwritten.
  ; The old build is quiesced first: its daemon is stopped, its ports are
  ; freed, and its data is left alone, because an update keeps the data. A
  ; build that predates --force is stopped by name, because an installer that
  ; refuses to update over a running daemon helps nobody.
  ${If} ${FileExists} "$INSTDIR\relo.exe"
    ClearErrors
    ExecWait '"$INSTDIR\relo.exe" daemon stop --force' $0
    ${If} $0 != 0
      nsExec::Exec 'taskkill /F /IM relo.exe /IM relo-desktop.exe'
    ${EndIf}
  ${EndIf}
  SetOutPath "$INSTDIR"
  File "/oname=relo.exe" "${BINARY}"
  SetOutPath "$INSTDIR\bin"
  File "/oname=relo.cmd" "${SHIMCMD}"
  File "/oname=relo" "${SHIMSH}"
  SetOutPath "$INSTDIR"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  Call AddToPath

  CreateDirectory "$SMPROGRAMS\Relo"
  ; The shortcut starts the daemon, which carries the tray icon. $OUTDIR is
  ; stored as the shortcut's working directory. The install directory is not
  ; a place to write, and an empty "Start in" would inherit Explorer's own
  ; directory at launch; the shell expands %USERPROFILE% for whoever runs the
  ; shortcut, so the working directory is always writable.
  StrCpy $OUTDIR "%USERPROFILE%"
  CreateShortCut "$SMPROGRAMS\Relo\Relo.lnk" "$INSTDIR\relo.exe" "daemon start" "$INSTDIR\relo.exe" 0
  StrCpy $OUTDIR "$INSTDIR"

  ; The start command spawns the daemon detached, which is what the app also
  ; registers for itself on first run; this covers the install that never
  ; starts it. relo.exe owns no console, so a login start stays quiet and
  ; the tray waits in the notification area.
  WriteRegStr HKCU "${RUN_KEY}" "ReloDaemon" '"$INSTDIR\relo.exe" daemon start'

  ; Builds from before the tray moved into the daemon registered "Relo" with
  ; a desktop command that no longer exists. Left behind, it fails at every
  ; login with nothing on screen to say so. Windows keeps a separate record
  ; of which login items the user switched off, which goes with it.
  DeleteRegValue HKCU "${RUN_KEY}" "Relo"
  DeleteRegValue HKCU "${APPROVED_KEY}" "Relo"

  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "Relo"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "Relo"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
SectionEnd

Section "Uninstall"
  ; The binary may already be gone -- `relo uninstall` deletes it -- so the
  ; prepare is best effort: it stops the service, frees the configured ports,
  ; and turns autostart off, and the uninstaller carries on either way.
  ${If} ${FileExists} "$INSTDIR\relo.exe"
    ClearErrors
    ExecWait '"$INSTDIR\relo.exe" uninstall --prepare' $0
  ${EndIf}
  Call un.AskAboutData
  Call un.RemoveFromPath
  Delete "$INSTDIR\relo.exe"
  Delete "$INSTDIR\bin\relo.cmd"
  Delete "$INSTDIR\bin\relo"
  RMDir "$INSTDIR\bin"
  ; An interrupted upgrade can leave the old GUI build behind.
  Delete "$INSTDIR\relo-desktop.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\Relo\Relo.lnk"
  RMDir "$SMPROGRAMS\Relo"
  DeleteRegValue HKCU "${RUN_KEY}" "Relo"
  ; The daemon login item is written at runtime, not by this installer, so
  ; uninstalling has to remove it itself.
  DeleteRegValue HKCU "${RUN_KEY}" "ReloDaemon"
  DeleteRegKey HKCU "${UNINST_KEY}"
SectionEnd

; CheckConflictingRelo refuses the install when another Relo owns the relo
; command. where.exe lists every relo on PATH; an entry under the install
; directory is this product's own update, while anything else is a previous
; npm or manual install the operator removes first, or keeps using instead
; of this one. The search runs from the system directory so a stray copy
; beside the installer is not mistaken for an install.
Function CheckConflictingRelo
  retry:
    Push $OUTDIR
    SetOutPath "$SYSDIR"
    nsExec::ExecToStack '"$SYSDIR\where.exe" relo'
    Pop $0
    Pop $1
    Pop $OUTDIR
    SetOutPath $OUTDIR
    ${If} $0 != "0"
      Return
    ${EndIf}
    FileOpen $2 "$PLUGINSDIR\relopaths.txt" w
    FileWrite $2 $1
    FileClose $2
    FileOpen $2 "$PLUGINSDIR\relopaths.txt" r
    StrCpy $3 ""
    conflict_loop:
      ClearErrors
      FileRead $2 $4
      ${If} ${Errors}
        Goto conflict_done
      ${EndIf}
      ${StrRep} $4 $4 "$\r" ""
      ${StrRep} $4 $4 "$\n" ""
      ${If} $4 == ""
        Goto conflict_loop
      ${EndIf}
      ; StrStr matches case-insensitively and returns the line unchanged
      ; when the match is at the start, which the case-sensitive
      ; comparison below can then test.
      ${StrStr} $5 $4 "$INSTDIR\"
      ${If} $5 == $4
        Goto conflict_loop
      ${EndIf}
      StrCpy $3 $4
    conflict_done:
    FileClose $2
    Delete "$PLUGINSDIR\relopaths.txt"
    ${If} $3 == ""
      Return
    ${EndIf}
    MessageBox MB_RETRYCANCEL|MB_ICONSTOP "Another Relo install provides the relo command:$\r$\n$\r$\n$3$\r$\n$\r$\nRemove it first to install here (an npm install goes with: npm uninstall -g relo; a Setup install goes in Settings under Apps), or keep using that install instead of this one.$\r$\n$\r$\nRetry checks again after the other install is gone." IDRETRY retry
    Abort
FunctionEnd

; AskAboutData asks whether the Relo data goes with the program. The question
; is skipped when `relo uninstall` already decided -- it passes /DATA=handled
; -- and when the uninstaller runs silently, where a question has nobody to
; answer it.
Function un.AskAboutData
  ${If} ${Silent}
    Return
  ${EndIf}
  ${UnStrStr} $0 "$CMDLINE" "/DATA=handled"
  ${If} $0 != ""
    Return
  ${EndIf}
  MessageBox MB_YESNO|MB_ICONQUESTION \
    "Remove your Relo data too (configuration, credentials, logs)?$\r$\n$\r$\nChoose No to keep it for a later install." \
    IDYES wipe IDNO keep
  wipe:
    ${If} ${FileExists} "$INSTDIR\relo.exe"
      ExecWait '"$INSTDIR\relo.exe" uninstall --prepare --wipe'
    ${EndIf}
    Return
  keep:
FunctionEnd

; AddToPath appends the shim's directory to the user PATH, unless it is
; already listed or the PATH is too long to edit safely.
Function AddToPath
  ReadRegStr $0 HKCU "${ENVIRONMENT_KEY}" "Path"
  StrLen $1 $0
  ${If} $1 >= ${PATH_LIMIT}
    DetailPrint "the user PATH is longer than ${PATH_LIMIT} characters; add $INSTDIR\bin by hand"
    MessageBox MB_ICONINFORMATION|MB_OK "Relo did not add its directory to PATH, because the PATH stored for your account is longer than this installer can read safely.$\r$\n$\r$\nAdd this directory by hand to run relo from a terminal:$\r$\n$INSTDIR\bin"
    Return
  ${EndIf}
  ${StrStr} $2 ";$0;" ";$INSTDIR\bin;"
  ${If} $2 != ""
    DetailPrint "$INSTDIR\bin is already on PATH"
    Return
  ${EndIf}
  ; A profile can have no user PATH at all, and a leading ";" would make an
  ; empty entry, which Windows resolves to the current directory.
  ${If} $0 == ""
    WriteRegExpandStr HKCU "${ENVIRONMENT_KEY}" "Path" "$INSTDIR\bin"
  ${Else}
    WriteRegExpandStr HKCU "${ENVIRONMENT_KEY}" "Path" "$0;$INSTDIR\bin"
  ${EndIf}
  DetailPrint "added $INSTDIR\bin to PATH"
  Call BroadcastEnvironmentChange
FunctionEnd

; RemoveFromPath drops the entry this installer added and nothing else, and
; keeps the REG_EXPAND_SZ type so %SystemRoot% entries keep expanding.
Function un.RemoveFromPath
  ReadRegStr $0 HKCU "${ENVIRONMENT_KEY}" "Path"
  StrLen $1 $0
  ${If} $1 >= ${PATH_LIMIT}
    DetailPrint "the user PATH is longer than ${PATH_LIMIT} characters; remove $INSTDIR\bin by hand"
    Return
  ${EndIf}
  ${UnStrStr} $2 ";$0;" ";$INSTDIR\bin;"
  ${If} $2 == ""
    Return
  ${EndIf}
  ; The list held nothing but this entry: drop the value instead of leaving a
  ; bare directory or an empty segment behind.
  ${If} $0 == "$INSTDIR\bin"
    DeleteRegValue HKCU "${ENVIRONMENT_KEY}" "Path"
    Call un.BroadcastEnvironmentChange
    Return
  ${EndIf}
  ${UnStrRep} $3 "$0" ";$INSTDIR\bin;" ";"
  ${UnStrRep} $3 "$3" ";$INSTDIR\bin" ""
  WriteRegExpandStr HKCU "${ENVIRONMENT_KEY}" "Path" "$3"
  Call un.BroadcastEnvironmentChange
FunctionEnd

; BroadcastEnvironmentChange asks running shells to re-read the environment,
; so a terminal opened after it sees the new PATH.
Function BroadcastEnvironmentChange
  System::Call 'user32::SendMessageTimeoutW(i 0xFFFF, i 0x001A, i 0, w "Environment", i 2, i 5000, *i .r0)'
FunctionEnd

Function un.BroadcastEnvironmentChange
  System::Call 'user32::SendMessageTimeoutW(i 0xFFFF, i 0x001A, i 0, w "Environment", i 2, i 5000, *i .r0)'
FunctionEnd
