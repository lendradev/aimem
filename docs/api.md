---
title: API dan CLI — AI Memory
date: 2026-11-21
status: draft
owner: Lendra
tags: [api, cli, mcp]
---
# API dan CLI — AI Memory

Dua antarmuka: CLI untuk manusia dan agent shell, MCP stdio server untuk agent tool. Keduanya memanggil engine yang sama.

## CLI

```
aimem remember <title> [flags]     simpan satu item
aimem recall <query> [flags]       cari memory
aimem show <id> [flags]            tampilkan satu item penuh
aimem forget <id>... [flags]       hapus item
aimem feedback <id> <outcome>      catat useful|dead_end|corrected
aimem promote [<id>] [flags]       pindahkan item terbukti ke scope global
aimem reflect [flags]              tulis LESSONS.md
aimem stats [flags]                jumlah baris dan ukuran database
aimem prune [flags]                buang item dan signal lama
aimem mcp                          jalankan MCP stdio server
aimem version
```

Flag global: `--db PATH` (override path database), `--json` (output machine readable).

Nama binary bisa `aimem`, `ai-memory`, `aimemory`, `ai-memory-cli`, atau `aimem-cli`. Semua nama memakai direktori data yang sama, jadi mengganti nama tidak memecah memory.

### Perintah

| Perintah | Flag | Default | Perilaku |
|---|---|---|---|
| `remember` | `--kind` | `fact` | `fact`, `decision`, `procedure`, `preference`, `lesson`, `dead_end`, `correction` |
| | `--body` | kosong | detail, maksimal 4096 byte |
| | `--scope` | `project` | `project` atau `global` |
| | `--tag` | kosong | repeatable, maksimal 8 tag, lowercase |
| | `--allow-duplicate` | false | simpan walau ada item yang mirip |
| `recall` | `--scope` | `all` | `all`, `project`, `global` |
| | `--limit` | `5` | jumlah item maksimum |
| | `--tokens` | `600` | budget token perkiraan |
| | `--kind` | semua | repeatable, filter jenis |
| | `--include-dead-ends` | false | sertakan item dead_end dan corrected |
| | `--full` | false | body penuh, abaikan batas token |
| `feedback` | `--correction` | kosong | isi koreksi saat outcome `corrected` |
| | `--source` | `cli` | sumber sinyal, dipakai untuk korroborasi |
| `promote` | `--list` | false | hanya tampilkan kandidat, tanpa memindahkan |
| `reflect` | `--out` | `<scope>/LESSONS.md` | path output |
| | `--scope` | `all` | scope yang direfleksi |
| `stats` | `--scope` | `all` | scope yang dihitung |
| `prune` | `--idle-days` | `180` | item tak terpakai lebih lama dari ini dibuang |
| | `--signal-days` | `365` | signal lebih lama dari ini dibuang |

Flag boleh muncul sebelum atau sesudah argumen posisional. `aimem remember judul --kind fact` dan `aimem remember --kind fact judul` setara.

### Batas

| Batas | Nilai |
|---|---|
| Panjang judul | 120 byte |
| Panjang body | 4096 byte |
| Jumlah tag | 8 |
| Panjang ID | 8 karakter |
| Similarity duplikat | 0,75 |

### Environment

| Variabel | Arti |
|---|---|
| `AI_MEMORY_HOME` | direktori untuk scope global. Default `~/.aimem` |
| `AI_MEMORY_PROJECT` | direktori untuk scope project. Default `<project-root>/.aimem` |

Project root dicari dari direktori kerja ke atas, cari `.git` lalu `go.mod`. Kalau tidak ada, direktori kerja dipakai.

### Output `stats`

Untuk manusia, ukuran database ditulis dengan satuan biner supaya angkanya tetap terbaca:

```console
$ aimem stats
project (/home/you/project/.aimem/data.db)
  size: 92.0 KiB
  items: 3
  signals: 0
  relations: 0
  embeddings: 0
```

Satuan yang dipakai: `B`, `KiB`, `MiB`, `GiB`, `TiB`, `PiB`. Nilai di bawah 1 KiB tetap ditampilkan sebagai byte bulat (`512 B`), di atasnya satu angka desimal.

Untuk mesin, `--json` tetap menyimpan jumlah byte mentah di `size_bytes`. Field itu tidak berubah:

```json
{
  "scope": "project",
  "path": "/home/you/project/.aimem/data.db",
  "rows": {
    "embeddings": 0,
    "items": 3,
    "relations": 0,
    "signals": 0
  },
  "size_bytes": 94208
}
```

Jadi output abbreviate hanya berlaku untuk output teks. Consumer JSON tidak terpengaruh dan tidak perlu parsing satuan.

## MCP server

JSON-RPC 2.0 over stdio, satu pesan JSON per baris. Protocol version `2024-11-05`.

### Methods

| Method | Hasil |
|---|---|
| `initialize` | protocolVersion, capabilities, serverInfo |
| `notifications/initialized` | tidak ada respons |
| `tools/list` | daftar tool beserta JSON Schema |
| `tools/call` | hasil tool |
| `ping` | objek kosong |

### Tools

**`memory_recall`**

| Input | Tipe | Default |
|---|---|---|
| `query` | string | wajib |
| `limit` | integer | 5 |
| `tokens` | integer | 600 |
| `scope` | `all` \| `project` \| `global` | `all` |
| `full` | boolean | false |

Output: `content` dengan satu blok `text`. Format tiap baris:

```
- [decision] pakai Go bukan Rust (id RJXJFTC3, score 1.85, scope project)
  keputusan user 2026-11-21
```

**`memory_remember`**

| Input | Tipe | Default |
|---|---|---|
| `title` | string | wajib |
| `body` | string | kosong |
| `kind` | salah satu dari 7 jenis | `fact` |
| `tags` | array string | kosong |
| `scope` | `project` \| `global` | `project` |

Output: `saved <id> (<kind>, scope <scope>)`.

**`memory_feedback`**

| Input | Tipe | Default |
|---|---|---|
| `id` | string | wajib |
| `outcome` | `useful` \| `dead_end` \| `corrected` | wajib |
| `correction` | string | kosong |

Output: `<id> -> status <status>, score <score>`.

### Contoh

```json
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"memory_recall","arguments":{"query":"keputusan go"}}}
```

## Model data

### Item

| Field | Tipe | Arti |
|---|---|---|
| `id` | string | 8 karakter, stable |
| `kind` | Kind | jenis item |
| `title` | string | satu kalimat, wajib, maks 120 byte |
| `body` | string | detail, maks 4 KB |
| `tags` | []string | lowercase, maks 8 |
| `scope` | `global` \| `project` | scope penyimpanan |
| `source` | string | `cli`, `mcp`, atau asal lain |
| `status` | Status | hasil derivasi dari signal |
| `score` | float | signed usefulness score, time-decayed |
| `use_count` | int | kali dipanggil |
| `created_at`, `updated_at`, `last_used` | time | unix seconds |

### Status

| Status | Arti |
|---|---|
| `tentative` | belum terkonfirmasi, satu sumber saja |
| `preferred` | minimal 2 sumber berbeda memberi sinyal `useful` |
| `contested` | ada sinyal positif dan negatif |
| `dead_end` | hanya sinyal negatif `dead_end` |
| `corrected` | hanya sinyal negatif `corrected` |
| `archived` | item yang diarsipkan |

### Outcome signal

| Outcome | Weight | Arti |
|---|---|---|
| `useful` | +1 | item membantu |
| `dead_end` | -1 | item tidak membantu, jangan diulang |
| `corrected` | -1.5 | item salah, ada jawaban yang benar |

Skor item: `sum(weight * 0.5 ^ (umur_hari / 30))`, dibulatkan 9 digit desimal supaya urutan stabil lintas platform.

### Skema SQLite

Tabel `items`, `signals`, `relations`, `embeddings`, plus dua virtual table FTS5:

| Virtual table | Tokenizer | Isi | Dipakai saat |
|---|---|---|---|
| `items_fts` | `porter unicode61 remove_diacritics 2` | title, body, tags | pencarian utama |
| `items_tri` | `trigram` | title, body | fallback, kalau `items_fts` tidak menemukan apa-apa |

Keduanya external-content (`content='items'`) dengan tiga trigger sinkron insert/update/delete.

`items_tri` ada karena FTS5 berbasis token butuh kecocokan token utuh, jadi query prefix seperti `pool` tidak menemukan `pooling`. Trigram menutup celah itu tanpa menambah dependency.

`embeddings` sengaja dibuat lebih awal dan default kosong, supaya pivot ke retrieval semantic tidak butuh migrasi tabel `items`.

## Mekanisme belajar

Empat mekanisme membuat store makin tajam seiring pemakaian, tanpa perlu agent melakukan apa pun secara eksplisit:

| Mekanisme | Aturan |
|---|---|
| Reinforcement | Setiap recall menambah `0,15` ke skor item, dengan half-life 45 hari dan plafon total 1,5. Bukti lemah, jadi tidak bisa mengalahkan item yang sudah terkonfirmasi. |
| Decay | Bobot outcome dan reinforcement sama-sama meluruh. Half-life outcome 30 hari, reinforcement 45 hari. |
| Deduplikasi | `remember` menghitung similarity (Jaccard dari token, digabung containment) antara judul+body baru dan item yang ada. Di atas 0,75 item tidak disimpan dan duplikatnya dilaporkan. |
| Promosi | Item project yang dikonfirmasi oleh minimal 2 sumber berbeda dan sudah dipanggil minimal 2 kali Become kandidat promosi ke scope global. `aimem promote <id>` memindahkannya, termasuk riwayat signalnya. |

Reinforcement diterapkan saat recall, jadi `use_count` dan `last_used` selalu mencerminkan pemakaian nyata.

## Query safety

Query user tidak pernah masuk `MATCH` sebagai string mentah. Pipeline: normalisasi (lowercase, pisah token, buang stopword) lalu setiap term di-quote dengan FTS5 escaping (`"` internal jadi `""`), digabung dengan `OR`. Query kosong menghasilkan 0 baris, bukan error.

Alasannya: query mentah seperti `go-build`, `a:b`, atau `(cache` memicu SQL error di parser FTS5. Sudah diverifikasi lewat spike FTS5 pure-Go.

Query natural language yang tidak berbagi token utuh dengan isi memory (mis. "bahasa apa yang dipakai" untuk memory yang berisi "dibangun dengan Go") **tidak** ditemukan oleh retrieval lexical. Ini batas yang diketahui, bukan bug. Solusinya fase semantic (isi tabel `embeddings`), bukan sekarang.

## Biaya storage dan token

Diukur dengan 200 item decision bertipe sama (judul ~60 byte, body ~200 byte):

| Ukuran | Nilai |
|---|---|
| Ukuran database | 504 KB untuk 200 item, sekitar 2,5 KB per item |
| Output recall (5 item, budget 600 token) | sekitar 1,4 KB, sekitar 350 token |
| Database kosong | 94 KB (page size minimum SQLite) |

Batas yang membatasi pertumbuhan:

| Batas | Nilai |
|---|---|
| Body per item | 4096 byte |
| Judul per item | 120 byte |
| Tag per item | 8 |
| Item tak terpakai | dibuang `aimem prune` setelah 180 hari |
| Signal lama | dibuang `aimem prune` setelah 365 hari |
| Ruam kosong | dikembalikan `auto_vacuum=INCREMENTAL` + `PRAGMA incremental_vacuum` |
| Item yang dipakai | `use_count` naik, `last_used` diperbarui, jadi prune tidak menyentuh yang aktif |

Item yang dihapus diprune berdasarkan `use_count = 0`, jadi item yang pernah dipakai recall tidak hilang hanya karena tidak dipakai dalam 180 hari. Ini kompromi yang disengaja: storage dijaga, tapi item aktif tidak hilang.
