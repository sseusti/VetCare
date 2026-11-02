# Здесь будет генератор данных клиники

## Запуск

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt

python generate.py      # CSV в data/raw/
pytest                  # тесты
ruff check . && ruff format --check .   # линтер и форматирование
```
