# AGENTS.md — aimem

Aturan untuk AI agent yang mengerjakan `aimem`. Baca sekali di awal sesi, lalu ikuti. Instruksi user di chat selalu menang kalau bertentangan dengan file ini; sebutkan itu dalam satu baris.

## 1. Struktur repo

```
AGENTS.md   README.md   cmd/   internal/   scripts/   docs/
```

- Source code Go ada di root repo ini. `cmd/aimem/` adalah entry point, `cmd/spike/` adalah spike verifikasi FTS5.
- `docs/` berisi dokumentasi yang dipublikasikan: `api.md` (kontrak CLI dan MCP), `install.md` (panduan install per OS), `agents.md` (setup MCP per AI agent), `README.md` (indeks).
- Plans, riset, dan ADR adalah dokumen kerja internal dan tidak ada di repo ini. Jangan mencari path-nya.
- Commit hanya jika diminta. Pesan commit mengikuti Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`).
- Butuh persetujuan eksplisit: push, force push, `reset --hard`, `rm -rf`, drop/hapus data, ganti/hapus file user, menambah dependency besar.
- Edit minimal dan terarah. Jangan menulis ulang file utuh atau memformat ulang kode yang tidak disentuh.
- Jangan pernah commit secret, token, `.env`, atau data memory pengguna.

## 2. Awal sesi (singkat)

1. Baca file ini.
2. Baca `docs/README.md` sebelum membuat dokumen baru, supaya tidak ada duplikasi.
3. Kalau tugasnya error, bug, atau kendala konfigurasi: cari dulu di dokumen dan issue yang relevan, baru debug.

Jangan memindai seluruh repo. Cari dengan target (nama file, simbol), baca bagian yang perlu saja.

## 3. Skill: kapan memakai yang mana

**WAJIB: pakai skill kalau ada yang cocok domain.** Skill berisi prosedur, gate, dan batasan yang kalau dilewati menghasilkan kerja yang salah (build rusak, keamanan bocor, dokumentasi kosong). Menulis ulang dari naluri hampir selalu lebih buruk daripada mengikuti skill yang sudah ada.

Aturan praktis:

1. **Cari dulu, baru kerja.**
2. **Satu skill utama per tugas.** Tambah yang kedua hanya kalau tugas melintasi domain.
3. **Sebut skill yang dipakai** di laporan akhir.
4. **Skill tidak ada atau tidak cocok** = lanjut manual, dan sebut di laporan.
5. **Tool lain** (Claude Code, dan sejenisnya): baca `SKILL.md` skill yang relevan dari folder skill dan ikuti isinya.

| Skill | Pakai saat | Keluaran → lokasi |
|---|---|---|
| `research-and-documentation` | membandingkan opsi, feasibility, spike, keputusan teknis, mendokumentasikan kode, audit docs | laporan → `docs/`; keputusan final → `docs/adr/` |
| `senior-backend-security` | API, database, auth, infra, performa, threat model, review keamanan, hardening | kode → `internal/`; kontrak API → `docs/api.md` |
| `security-review` | audit keamanan source-first: review diff/PR atau full audit | temuan → `docs/`; perbaikan kode → `internal/` |
| `karpathy-guidelines` | menulis, review, atau refactor kode: hindari over-kompleksitas, patch minimal, asumsi eksplisit, kriteria sukses terverifikasi | panduan di `internal/`, review sebelum commit |
| `dockerize-project` | membuat Dockerfile, docker-compose, .env.example untuk project yang sudah ada | artefak container → root repo |
| `code-agents/*` (pack pendukung) | lapisan disiplin kerja: recon, patch minimal, validasi, guard diff | output ke `internal/` |

Urutan prioritas kalau skill berbeda pendapat: instruksi user → `AGENTS.md` → keamanan dan privasi data → aksesibilitas → kebutuhan produk → preferensi estetika.

## 4. Dokumentasi (`docs/`)

- Format Markdown, bahasa Inggris untuk dokumen publik repo ini.
- Front matter pada dokumen baru: `title`, `date`, `status`, `owner`, `tags`.
- Indeks: selalu perbarui `docs/README.md` setelah menambah, mengganti nama, atau mengubah status dokumen.
- Status dokumen baru: `draft`. Dokumen yang digantikan diberi `superseded` dan ditautkan dua arah, tidak dihapus.
- Jangan menautkan dokumen internal dari dokumen publik.

## 5. Stack dan perintah

| Item | Nilai |
|---|---|
| Bahasa | Go 1.26, module `github.com/lendradev/aimem` |
| Package manager | `go mod` |
| Install | `./scripts/build.sh --install` (binary ke `dist/aimem`, lalu `~/.local/bin/aimem`) |
| Build | `./scripts/build.sh` |
| Lint / type-check | `go vet ./...` + `gofmt -l .` |
| Test | `go test ./...` |
| Database | SQLite via `modernc.org/sqlite` (pure Go, FTS5). Tidak ada service eksternal |
| UI | tidak ada; output CLI dan MCP |
| Target platform | Linux, macOS, Windows; binary statis, tanpa cgo |
| Kepatuhan | data lokal di device, tidak ada telemetry, tidak ada data pribadi nyata di repo |

Perintah lain yang berlaku:

- `scripts/go.sh` menjalankan `go` native kalau Go ada di `PATH`, dan di container Docker kalau tidak. Cache toolchain dipakai ulang antar run.
  ```bash
  ./scripts/go.sh build ./...
  ./scripts/go.sh test ./...
  ./scripts/go.sh vet ./...
  ./scripts/go.sh sh -c 'gofmt -l .'
  ./scripts/go.sh run ./cmd/spike
  ./scripts/build.sh --install
  ```
- Binary output ke `dist/aimem`, tidak di-commit (`dist/` ada di `.gitignore`).
- Data memory runtime tidak pernah masuk repo. `.aimem/` ada di `.gitignore`.
- Test tidak boleh menulis ke `~/.aimem` sungguhan. Pakai `AI_MEMORY_HOME` dan `AI_MEMORY_PROJECT` ke temp dir.

## 6. Aturan implementasi

Aturan detail ada di `CONTRIBUTING.md`. Yang paling sering dilanggar:

1. **Deterministik.** Retrieval, scoring, dan refleksi menghasilkan output sama untuk input dan clock sama. Tidak ada panggilan LLM, tidak ada jaringan.
2. **Jangan pernah kirim input user mentah ke parser FTS5.** Escape di `internal/store/query.go`. Query mentah memicu SQL error dan, lebih buruk, mengubah arti pencarian.
3. **Batasi setiap input.** Panjang judul, panjang body, jumlah tag, dan budget token punya batas eksplisit. Batas itu harus sinkron dengan `docs/api.md`.
4. **Tokenizer aman untuk script.** Script tanpa pemisah kata (Mandarin, Jepang, Korea, Thai) harus tetap utuh, dan combining mark tidak boleh jadi pemisah. Regresi di sini diam-diam merusak recall untuk bahasa itu.
5. **Tambah test setiap perubahan perilaku.** Suite-nya cepat, tidak ada alasan cabang tanpa coverage.
6. **Jangan sentuh memory store user sungguhan.**

## 7. Standar selesai

- Build, lint, type-check, dan tes yang relevan lulus. Kalau tidak bisa dijalankan, katakan.
- Perubahan perilaku disertai tes.
- `docs/` dan status rencana sudah diperbarui.
- Tidak ada secret, kredensial, atau data pribadi nyata di kode, log, docs, atau commit.
- Skill yang dipakai disebut di laporan akhir.

## 8. Laporan akhir ke user

Singkat (3 sampai 6 baris): apa yang selesai, path file yang dibuat/diubah, hasil verifikasi, asumsi yang diambil, skill yang dipakai, dan satu langkah berikutnya yang konkret.
