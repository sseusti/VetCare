# VetCare

Информационная система ветеринарной клиники: от модели данных до аналитики.

| Часть | Что внутри | Статус |
| --- | --- | --- |
| [docs/lab1](docs/lab1) | Предметная область, UML и ER-диаграммы | готово |
| [storage-engine](storage-engine) | Файловая БД «Приёмы» на Go + Fyne с хеш-индексами ([отчёт](docs/lab2/report.md)) | готово |

## Быстрый старт

```bash
cd storage-engine
go run ./cmd/vetdb
```

Для сборки на macOS нужны Go 1.24+ и Xcode Command Line Tools (`xcode-select --install`).
