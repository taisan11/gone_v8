[CmdletBinding()]
param(
    [string]$Clang = "clang-cl",
    [switch]$SkipFetch
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
if (-not [Environment]::Is64BitOperatingSystem) {
    throw "The prebuilt V8 library requires Windows x64."
}
if (-not $env:INCLUDE -or -not $env:LIB) {
    throw "Run this script from an x64 Visual Studio Developer PowerShell (C++ Build Tools and Windows SDK required)."
}
if ($env:VSCMD_ARG_TGT_ARCH -and $env:VSCMD_ARG_TGT_ARCH -ne "x64") {
    throw "Select the x64 developer environment, not $env:VSCMD_ARG_TGT_ARCH."
}
$compiler = Get-Command $Clang -CommandType Application -ErrorAction SilentlyContinue
if (-not $compiler -and $Clang -eq "clang-cl") {
    # Developer PowerShell does not always add optional LLVM tools to PATH.
    $candidates = @()
    if ($env:ProgramFiles) {
        $candidates += Join-Path $env:ProgramFiles "LLVM/bin/clang-cl.exe"
    }
    if ($env:VSINSTALLDIR) {
        $candidates += Join-Path $env:VSINSTALLDIR "VC/Tools/Llvm/x64/bin/clang-cl.exe"
        $candidates += Join-Path $env:VSINSTALLDIR "VC/Tools/Llvm/bin/clang-cl.exe"
    }
    $vswhere = Join-Path ${env:ProgramFiles(x86)} "Microsoft Visual Studio/Installer/vswhere.exe"
    if (Test-Path -LiteralPath $vswhere) {
        $candidates += & $vswhere -all -products '*' -find 'VC\Tools\Llvm\**\clang-cl.exe'
    }
    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            $compiler = Get-Command $candidate -CommandType Application -ErrorAction Stop
            break
        }
    }
}
if (-not $compiler) {
    throw @"
Cannot find '$Clang'. Developer PowerShell alone does not install Clang.
Open Visual Studio Installer > Build Tools > Modify > Individual components,
and install 'C++ Clang tools for Windows', then reopen Developer PowerShell.
Alternatively install LLVM and pass its full path:
  ./tools/build-windows.ps1 -Clang 'C:\Program Files\LLVM\bin\clang-cl.exe'
"@
}
Write-Host "Using compiler: $($compiler.Source)"

Push-Location $root
try {
    if (-not $SkipFetch) {
        & go run ./tools/fetch.go -target windows/amd64
        if ($LASTEXITCODE -ne 0) { throw "Fetching V8 dependencies failed ($LASTEXITCODE)." }
    }
    $deps = Join-Path $root "deps/windows_amd64"
    $libcxx = Join-Path $deps "libcxx"
    $required = @(
        "$root/deps/include/v8.h",
        "$deps/libv8.lib",
        "$libcxx/third_party/libc++/src/include/__config",
        "$libcxx/buildtools/third_party/libc++/__config_site"
    )
    foreach ($path in $required) {
        if (-not (Test-Path -LiteralPath $path)) { throw "Missing $path; run go run ./tools/fetch.go first." }
    }
    $headersVersion = (Get-Content "$root/deps/.headers-version" -Raw).Trim()
    $libraryVersion = (Get-Content "$deps/libv8.lib.version" -Raw).Trim()
    $libcxxVersion = (Get-Content "$libcxx/.version" -Raw).Trim()
    if ($headersVersion -ne $libraryVersion -or $headersVersion -ne $libcxxVersion) {
        throw "V8 headers, library and libc++ versions differ; fetch the same -version for all dependencies."
    }
    # Compile into a staging directory so a failed link cannot replace a good DLL.
    $stage = Join-Path $deps (".bridge-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $stage | Out-Null
    try {
        $arguments = @(
            "/nologo", "/c", "/O2", "/MT", "/GR-", "/std:c++20",
            "/clang:-fno-exceptions", "/clang:-nostdinc++",
            "/DWIN32_LEAN_AND_MEAN", "/DNOMINMAX", "/D_CRT_SECURE_NO_WARNINGS",
            "/DV8_COMPRESS_POINTERS", "/DV8_31BIT_SMIS_ON_64BIT_ARCH",
            "/D_LIBCPP_HARDENING_MODE=_LIBCPP_HARDENING_MODE_NONE",
            "/D_LIBCPP_DISABLE_VISIBILITY_ANNOTATIONS",
            "/I$root/deps/include",
            "/I$libcxx/third_party/libc++/src/include",
            "/I$libcxx/buildtools/third_party/libc++",
            "/Fo$stage/bridge.obj", "$root/native/windows/bridge.cc"
        )
        & $compiler.Source @arguments
        if ($LASTEXITCODE -ne 0) {
            throw "Bridge compilation failed ($LASTEXITCODE). Use a recent LLVM clang-cl compatible with the bundled Chromium libc++ headers."
        }
        # Chromium libc++ already supplies std::nothrow and
        # std::uncaught_exceptions. Linking the entire MSVC STL archive can
        # select its competing definitions before libc++'s objects are loaded.
        # libc++ only needs MSVC's __ExceptionPtr* implementation here. Extract
        # that object without modifying the installed library or suppressing
        # duplicate-symbol errors with /FORCE:MULTIPLE.
        $librarian = Get-Command lib.exe -CommandType Application -ErrorAction Stop
        $libcpmt = $null
        foreach ($directory in ($env:LIB -split ';')) {
            if (-not $directory) { continue }
            $candidate = Join-Path $directory 'libcpmt.lib'
            if (Test-Path -LiteralPath $candidate -PathType Leaf) {
                $libcpmt = $candidate
                break
            }
        }
        if (-not $libcpmt) { throw 'Cannot find x64 libcpmt.lib in the developer environment LIB path.' }
        $members = @(& $librarian.Source /nologo /list $libcpmt)
        if ($LASTEXITCODE -ne 0) { throw "Listing libcpmt.lib failed ($LASTEXITCODE)." }
        $exceptionMembers = @($members | Where-Object { $_ -match '(^|[\\/])excptptr\.obj$' })
        if ($exceptionMembers.Count -ne 1) {
            throw 'Expected one excptptr.obj in libcpmt.lib; this MSVC runtime layout is not supported.'
        }
        & $librarian.Source /nologo "/extract:$($exceptionMembers[0])" "/out:$stage/msvc-excptptr.obj" $libcpmt
        if ($LASTEXITCODE -ne 0) { throw "Extracting MSVC exception support failed ($LASTEXITCODE)." }

        $arguments = @(
            "/nologo", "/LD", "/MT", "$stage/bridge.obj", "$stage/msvc-excptptr.obj", "$deps/libv8.lib",
            "/link", "/MACHINE:X64", "/DEF:$root/native/windows/bridge.def",
            "/OUT:$stage/gone_v8.dll", "/IMPLIB:$stage/gone_v8.lib",
            "/NODEFAULTLIB:libcpmt.lib", "winmm.lib", "dbghelp.lib", "bcrypt.lib",
            "advapi32.lib", "user32.lib", "shell32.lib", "ole32.lib", "ws2_32.lib"
        )
        & $compiler.Source @arguments
        if ($LASTEXITCODE -ne 0) { throw "Bridge linking failed ($LASTEXITCODE)." }
        Copy-Item -LiteralPath "$stage/gone_v8.dll" -Destination "$deps/gone_v8.dll" -Force
    }
    finally {
        Remove-Item -LiteralPath $stage -Recurse -Force
    }
    Write-Host "Built $deps/gone_v8.dll"
    Write-Host 'Next: go test ./...; go run ./cmd/demo (CGO is not required on Windows).'
}
finally {
    Pop-Location
}
