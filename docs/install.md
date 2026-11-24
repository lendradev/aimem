---
title: Panduan Install aimem
date: 2026-11-21
status: draft
owner: Lendra
tags: [install, setup, mcp]
---
# Panduan Install aimem

`aimem` adalah binary tunggal tanpa runtime dependency. Pasang sekali, lalu hubungkan ke AI agent lewat MCP.

Dokumen ini mencakup install per OS. Untuk konfigurasi MCP per agent, lihat [agents.md](agents.md).

## Ringkas

| Langkah | Apa yang terjadi |
|---|---|
| 1. Ambil binary | unduh file sesuai OS dan arsitektur |
| 2. Verifikasi checksum | cocokkan dengan `SHA256SUMS` |
| 3. Pasang ke `PATH` | binary bisa dipanggil dari mana saja |
| 4. Cek | `aimem version` menampilkan versi |
| 5. Hubungkan MCP | lihat [agents.md](agents.md) |

Binary tersedia untuk: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`, `windows/arm64`.

Format nama file release:

```
aimem_linux_amd64
aimem_linux_arm64
aimem_darwin_amd64
aimem_darwin_arm64
aimem_windows_amd64.exe
aimem_windows_arm64.exe
```

---

## Linux

Deteksi arsitektur:

```bash
uname -m
```

`x86_64` berarti `amd64`, `aarch64` berarti `arm64`.

### amd64

```bash
cd /tmp
curl -fsSLO https://github.com/lendradev/aimem/releases/latest/download/aimem_linux_amd64
curl -fsSLO https://github.com/lendradev/aimem/releases/latest/download/SHA256SUMS
```

### arm64

```bash
cd /tmp
curl -fsSLO https://github.com/lendradev/aimem/releases/latest/download/aimem_linux_arm64
curl -fsSLO https://github.com/lendradev/aimem/releases/latest/download/SHA256SUMS
```

### Verifikasi checksum

`--ignore-missing` membuat `sha256sum` hanya mengecek file yang ada di folder.

```bash
cd /tmp && sha256sum -c --ignore-missing SHA256SUMS
```

Output harus `aimem_linux_*: OK`. Kalau `FAILED`, jangan pakai file itu, unduh ulang.

### Pasang

```bash
sudo install -m 0755 /tmp/aimem_linux_amd64 /usr/local/bin/aimem
```

Ganti `aimem_linux_amd64` dengan `aimem_linux_arm64` kalau itu yang kamu unduh.

Tanpa `sudo`, pakai direktori user:

```bash
mkdir -p ~/.local/bin
install -m 0755 /tmp/aimem_linux_amd64 ~/.local/bin/aimem
```

Pastikan `~/.local/bin` ada di `PATH`:

```bash
grep -q 'HOME/.local/bin' ~/.bashrc || echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

### Cek

```bash
aimem version
```

Output: `aimem 0.3.1`.

---

## macOS

```bash
uname -m
```

`arm64` = Apple Silicon, `x86_64` = Intel.

### Apple Silicon

```bash
cd /tmp
curl -fsSLO https://github.com/lendradev/aimem/releases/latest/download/aimem_darwin_arm64
```

### Intel

```bash
cd /tmp
curl -fsSLO https://github.com/lendradev/aimem/releases/latest/download/aimem_darwin_amd64
```

### Verifikasi checksum

```bash
cd /tmp
curl -fsSLO https://github.com/lendradev/aimem/releases/latest/download/SHA256SUMS
sha256sum -c --ignore-missing SHA256SUMS
```

Output harus `aimem_darwin_*: OK`. Kalau `FAILED`, unduh ulang.

macOS punya `shasum` bawaan kalau `sha256sum` tidak ada:

```bash
shasum -a 256 -c SHA256SUMS
```

### Pasang

```bash
mkdir -p ~/bin
install -m 0755 /tmp/aimem_darwin_arm64 ~/bin/aimem
```

Ganti `aimem_darwin_arm64` dengan `aimem_darwin_amd64` kalau itu yang kamu unduh.

Tambahkan ke `PATH` (zsh):

```bash
grep -q 'HOME/bin' ~/.zshrc || echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

### Cek

```bash
aimem version
```

Binary `aimem` belum notarized Apple, jadi macOS bisa meminta izin saat pertama kali menjalankan file dari luar App Store. Jalankan sekali lewat Finder (klik kanan, Open), atau hilangkan quarantine flag:

```bash
xattr -d com.apple.quarantine /tmp/aimem_darwin_arm64 2>/dev/null || true
```

---

## Windows

Cek arsitektur di PowerShell:

```powershell
[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
```

`X64` = `amd64`, `Arm64` = `arm64`.

### Unduh

```powershell
New-Item -ItemType Directory -Force -Path "$env:TEMP\aimem" | Out-Null
Invoke-WebRequest -Uri "https://github.com/lendradev/aimem/releases/latest/download/aimem_windows_amd64.exe" -OutFile "$env:TEMP\aimem\aimem.exe"
```

Ganti `amd64` dengan `arm64` kalau perlu.

Windows menandai file dari internet sebagai diblokir. Lepas penanda itu:

```powershell
Unblock-File "$env:TEMP\aimem\aimem.exe"
```

### Verifikasi checksum

```powershell
Invoke-WebRequest -Uri "https://github.com/lendradev/aimem/releases/latest/download/SHA256SUMS" -OutFile "$env:TEMP\aimem\SHA256SUMS"
$line = (Select-String -Path "$env:TEMP\aimem\SHA256SUMS" -SimpleMatch "aimem_windows_amd64.exe").Line
$expected = $line.Split()[0].ToLower()
$actual = (Get-FileHash "$env:TEMP\aimem\aimem.exe" -Algorithm SHA256).Hash.ToLower()
if ($expected -ne $actual) { throw "checksum mismatch: $actual != $expected" }
"checksum ok"
```

Ganti `aimem_windows_amd64.exe` di `-SimpleMatch` kalau kamu unduh versi arm64.

### Pasang

Per-user, tanpa administrator:

```powershell
$bin = "$env:LOCALAPPDATA\Programs\aimem"
New-Item -ItemType Directory -Force -Path $bin | Out-Null
Copy-Item "$env:TEMP\aimem\aimem.exe" "$bin\aimem.exe"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
[Environment]::SetEnvironmentVariable("Path", "$userPath;$bin", "User")
```

Tutup lalu buka ulang terminal, atau jalankan ini di sesi sekarang:

```powershell
$env:Path += ";$env:LOCALAPPDATA\Programs\aimem"
```

### Cek

```powershell
aimem version
```

Kalau `aimem` tidak dikenali, pakai path penuh:

```powershell
& "$env:LOCALAPPDATA\Programs\aimem\aimem.exe" version
```

---

## Alternatif: install dengan Go

Butuh Go 1.26 atau lebih baru.

```bash
go install github.com/lendradev/aimem/cmd/aimem@latest
```

Binary masuk ke `$(go env GOPATH)/bin`. Pastikan folder itu ada di `PATH`.

## Alternatif: build dari source

```bash
git clone https://github.com/lendradev/aimem.git
cd aimem
./scripts/build.sh --install
```

Script memasang ke `~/.local/bin` (Linux), `~/bin` (macOS), atau `%LOCALAPPDATA%\Programs\aimem` (Windows). Target bisa diganti lewat `AIMEM_INSTALL_DIR`.

Kalau Go tidak ada tapi Docker ada, `scripts/go.sh` otomatis memakai container `golang:1.26.10-alpine` dan cache toolchain antar run.

---

## Setelah install

Cek cepat:

```bash
aimem remember "aimem install succeeded" --kind fact --body "Binary v0.3.1 is on PATH." --tag setup
aimem recall "install"
```

Memory disimpan di dua scope:

| Scope | Path | Isi |
|---|---|---|
| Project | `<project-root>/.aimem/data.db` | keputusan, prosedur, config repo ini |
| Global | `~/.aimem/data.db` | preferensi dan pelajaran lintas repo |

Semua nama binary (`aimem`, `ai-memory`, `aimemory`, `ai-memory-cli`, `aimem-cli`) memakai direktori `.aimem` yang sama, jadi mengganti nama tidak memecah memory.

Langkah berikutnya: hubungkan ke agent lewat MCP di [agents.md](agents.md).

---

## Troubleshooting

**`aimem: command not found`**
Binary ada tapi foldernya tidak di `PATH`. Cek dengan `command -v aimem`, lalu tambahkan folder install ke `PATH` dan buka shell baru.

**`cannot execute binary file: Exec format error`**
Arsitektur salah. Unduh ulang sesuai `uname -m` (Linux/macOS) atau `OSArchitecture` (Windows).

**`Permission denied` saat menjalankan**
Linux/macOS: `chmod +x /path/to/aimem`. Windows: file dari internetzone diblokir, jalankan `Unblock-File`.

**`database is locked`**
Satu proses menulis per database. Tutup agent lain yang sedang pakai `aimem` pada scope yang sama.

**MCP tidak muncul di agent**
`aimem mcp` harus bisa dijalankan tanpa error, dan path binary harus absolut di konfigurasi MCP. Lihat [agents.md](agents.md).

**Mau uninstall**

```bash
rm ~/.local/bin/aimem   # Linux
rm ~/bin/aimem           # macOS
```

Windows:

```powershell
Remove-Item "$env:LOCALAPPDATA\Programs\aimem\aimem.exe"
```

Data memory tidak ikut terhapus. Hapus manual kalau memang mau bersih: `~/.aimem/` (global) dan `<project-root>/.aimem/` (project).
