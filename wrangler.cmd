@echo off
set "WRANGLER_INVOCATION_DIR=%CD%"
if defined WSLENV (set "WSLENV=WRANGLER_INVOCATION_DIR/p:%WSLENV%") else (set "WSLENV=WRANGLER_INVOCATION_DIR/p")
pushd "%~dp0"
bash wrangler %*
popd