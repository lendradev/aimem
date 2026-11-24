---
title: Dokumentasi aimem
date: 2026-11-21
status: draft
---
# Dokumentasi aimem

| Dokumen | Isi |
|---|---|
| [install.md](install.md) | Panduan install per OS (Linux, macOS, Windows), verifikasi checksum, troubleshooting |
| [agents.md](agents.md) | Prompt siap salin per OS untuk pasang MCP, konfigurasi MCP per AI agent, blok aturan agent |
| [api.md](api.md) | Kontrak CLI dan MCP server, model data, skema SQLite, biaya storage dan token |
| [../README.md](../README.md) | Motivasi, panduan install dan compile, batasan, cara melapor bug |
| [../CONTRIBUTING.md](../CONTRIBUTING.md) | Contributor guide |

## Mulai cepat

```bash
aimem remember "judul" --kind decision --body "detail" --tag tag
aimem recall "query"
aimem feedback <id> useful
aimem reflect
```

Pasang binary di [install.md](install.md), lalu hubungkan ke AI agent di [agents.md](agents.md).

## Model ringkas

Memory item punya satu jenis (`fact`, `decision`, `procedure`, `preference`, `lesson`, `dead_end`, `correction`), judul satu kalimat, body sampai 32 KB, dan tag lowercase. Setiap item berada di satu scope: project (`<project-root>/.aimem/`) atau global (`~/.aimem/`).

Item fundamental adalah ilmu yang berlaku lintas project dan lintas tahun. Item ini ditandai saat disimpan (`--fundamental` atau `fundamental: true`) dan tidak pernah dibuang prune. Item lain adalah memory biasa: kalau tidak pernah dipanggil ulang dalam lima tahun, prune anggap stale dan buang.

Recall menggabungkan lexical match (FTS5 dengan porter stemming) dengan skor outcome. Item yang diberi `useful` naik skor dan statusnya jadi `preferred` setelah dikonfirmasi dua sumber. Item `dead_end` dan `corrected` disembunyikan dari recall default.

Isi item aimem ditulis dalam English (judul dan body), istilah teknis tetap apa adanya. Dokumentasi repo, README, komentar kode, dan output CLI ke user boleh English atau Bahasa Indonesia.


Detail lengkap ada di [api.md](api.md).
