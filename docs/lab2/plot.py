"""Draws bench.png from bench.csv for the lab report.

Usage: python3 plot.py   (run from docs/lab2 after cmd/bench)
"""
import csv
from collections import defaultdict

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402

SURFACE = "#fcfcfb"
INK = "#0b0b0b"
INK_2 = "#52514e"
GRID = "#e4e3df"
SERIES = ["#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4"]

INDEXED = [
    ("get_by_id", "Поиск по ID"),
    ("find_doctor_k10", "Поиск по врачу (k = 10)"),
    ("delete_by_id", "Удаление по ID"),
    ("insert", "Добавление"),
    ("update", "Редактирование"),
]
SEARCH = [  # operation, label, vertical offset of the end label in points
    ("find_doctor_k10", "Вторичный индекс, k = 10: O(1 + k)", -3),
    ("find_species", "Вторичный индекс, k ≈ N/10: O(1 + k)", -12),
    ("find_pet_name", "Полный проход: O(n)", 6),
]


def load(path="bench.csv"):
    data = defaultdict(dict)
    with open(path, newline="") as f:
        for row in csv.DictReader(f):
            data[row["operation"]][int(row["records"])] = int(row["ns_per_op"])
    return data


def style(ax, title, ylabel):
    ax.set_facecolor(SURFACE)
    ax.set_xscale("log")
    ax.set_title(title, loc="left", fontsize=12, color=INK, pad=12)
    ax.set_xlabel("Записей в базе", color=INK_2)
    ax.set_ylabel(ylabel, color=INK_2)
    ax.grid(True, which="major", color=GRID, linewidth=0.8)
    ax.tick_params(colors=INK_2)
    for side in ("top", "right"):
        ax.spines[side].set_visible(False)
    for side in ("left", "bottom"):
        ax.spines[side].set_color(GRID)


def main():
    data = load()
    sizes = sorted(data["insert"])
    fig, (left, right) = plt.subplots(1, 2, figsize=(13, 5), facecolor=SURFACE)

    for (op, label), color in zip(INDEXED, SERIES):
        ys = [data[op][n] / 1000 for n in sizes]
        left.plot(sizes, ys, color=color, linewidth=2, marker="o", markersize=7,
                  markeredgecolor=SURFACE, markeredgewidth=1.5, label=label)
    style(left, "Операции через индексы не растут с размером базы", "мкс на операцию")
    left.set_ylim(bottom=0)
    left.legend(frameon=False, labelcolor=INK, fontsize=9, loc="upper left")

    for (op, label, dy), color in zip(SEARCH, SERIES[:3]):
        ys = [data[op][n] / 1000 for n in sizes]
        right.plot(sizes, ys, color=color, linewidth=2, marker="o", markersize=7,
                   markeredgecolor=SURFACE, markeredgewidth=1.5, label=label)
        right.annotate(f"{ys[-1]:,.0f} мкс".replace(",", " "), (sizes[-1], ys[-1]),
                       textcoords="offset points", xytext=(8, dy), fontsize=9, color=INK)
    style(right, "Полный проход растёт линейно, индекс — нет", "мкс на поиск (лог. шкала)")
    right.set_yscale("log")
    right.set_xlim(right=sizes[-1] * 4)
    right.legend(frameon=False, labelcolor=INK, fontsize=9, loc="upper left")

    fig.tight_layout(w_pad=4)
    fig.savefig("bench.png", dpi=160, facecolor=SURFACE)
    print("saved bench.png")


if __name__ == "__main__":
    main()
