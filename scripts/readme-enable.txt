Running .ps1 files is disabled in the clean-room VM
===================================================

A fresh Windows client ships with the execution policy set to Restricted, so
the guest refuses to run readme-setup.ps1 and reports:

  ... cannot be loaded because running scripts is disabled on this system.

That is the default a real student would hit, not a fault in the script.
Two ways past it.

1. Bypass for that one command (nothing about the guest changes)

     powershell -ExecutionPolicy Bypass -File C:\Users\Public\readme-setup.ps1

   Add -Run to chain straight into the README walkthrough:

     powershell -ExecutionPolicy Bypass -File C:\Users\Public\readme-setup.ps1 -Run

2. Bypass for the current PowerShell window, then call the script directly

     Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
     .\readme-setup.ps1

   -Scope Process is the one to use: it dies with the window, so the clean room
   stays clean and the next attempt still meets the same default a student
   meets. -Scope CurrentUser and -Scope LocalMachine persist for the life of
   the guest, which hides this step from every run until the next
   vm-checkpoint-restore.ps1.
