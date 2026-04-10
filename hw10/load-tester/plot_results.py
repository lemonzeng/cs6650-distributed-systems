#!/usr/bin/env python3
"""
Plot load-test results for the report.

Produces three graphs per CSV (per config × ratio run):
  1. Read latency distribution  (histogram + CDF showing long tail)
  2. Write latency distribution (histogram + CDF showing long tail)
  3. Write->read time gap distribution (paired reads only)

Also produces a summary figure overlaying all configs for each ratio.

Usage
-----
# Single file
python plot_results.py results/w5r1_w1r99.csv

# Whole directory (produces per-file PNGs + summary figures)
python plot_results.py results/
"""

import argparse
import csv
import sys
from collections import defaultdict
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

# ---------------------------------------------------------------------------
# Data loading
# ---------------------------------------------------------------------------

def load_csv(path):
    records = []
    with open(path, newline="") as f:
        for row in csv.DictReader(f):
            try:
                row["latency_ms"] = float(row["latency_ms"])
            except (ValueError, KeyError):
                continue
            # stale: may be True/False/""
            s = row.get("stale", "")
            row["stale"] = (s == "True")
            # gap
            g = row.get("write_read_gap_ms", "")
            row["write_read_gap_ms"] = float(g) if g not in ("", "None") else None
            records.append(row)
    return records


# ---------------------------------------------------------------------------
# Plot helpers
# ---------------------------------------------------------------------------

def _hist_cdf(ax_hist, ax_cdf, latencies, color, label):
    """Draw histogram (log-Y) and CDF side by side."""
    if not latencies:
        return
    bins = 50
    ax_hist.hist(latencies, bins=bins, color=color, alpha=0.75,
                 edgecolor="white", label=label)
    ax_hist.set_yscale("log")
    ax_hist.yaxis.set_major_formatter(ticker.ScalarFormatter())
    ax_hist.set_xlabel("Latency (ms)")
    ax_hist.set_ylabel("Count (log scale)")
    ax_hist.legend(fontsize=8)

    # CDF
    s = sorted(latencies)
    n = len(s)
    cdf = [(i + 1) / n for i in range(n)]
    ax_cdf.plot(s, cdf, color=color, linewidth=1.8, label=label)
    ax_cdf.set_xlabel("Latency (ms)")
    ax_cdf.set_ylabel("Cumulative fraction")
    ax_cdf.set_ylim(0, 1.05)
    ax_cdf.grid(True, linestyle="--", alpha=0.4)
    ax_cdf.legend(fontsize=8)

    # mark p50, p95, p99
    for pct, ls in [(50, "--"), (95, ":"), (99, "-.")]:
        idx = int(n * pct / 100)
        val = s[min(idx, n - 1)]
        ax_cdf.axvline(val, color=color, linestyle=ls, alpha=0.6,
                       label=f"p{pct}={val:.0f}ms")
    ax_cdf.legend(fontsize=7)


def _gap_hist(ax, gaps, title):
    if not gaps:
        ax.text(0.5, 0.5, "No paired-read gap data",
                transform=ax.transAxes, ha="center", va="center", fontsize=11)
        ax.set_title(title)
        return
    ax.hist(gaps, bins=50, color="steelblue", edgecolor="white", alpha=0.85)
    ax.set_xlabel("Write → Read gap (ms)")
    ax.set_ylabel("Count")
    ax.set_title(title)
    avg = sum(gaps) / len(gaps)
    ax.axvline(avg, color="red", linestyle="--", label=f"mean={avg:.0f}ms")
    ax.legend(fontsize=8)


# ---------------------------------------------------------------------------
# Per-file figure  (3 graph panels: read lat, write lat, gap)
# ---------------------------------------------------------------------------

def plot_file(csv_path, out_dir):
    records = load_csv(csv_path)
    if not records:
        print(f"  (empty) {csv_path.name}")
        return

    writes = [r for r in records if r["op_type"] == "write"]
    reads  = [r for r in records if r["op_type"] == "read"]
    gaps   = [r["write_read_gap_ms"] for r in reads
              if r["write_read_gap_ms"] is not None]
    stale  = sum(1 for r in reads if r["stale"])

    w_lats = [r["latency_ms"] for r in writes]
    r_lats = [r["latency_ms"] for r in reads]

    stem  = csv_path.stem   # e.g. "w5r1_w1r99"
    title = stem.replace("_", "  ").upper()

    # 3 rows × 2 cols: (hist, cdf) for reads / writes / gap+info
    fig, axes = plt.subplots(3, 2, figsize=(13, 12))
    fig.suptitle(f"Load Test — {title}", fontsize=13, fontweight="bold")

    # --- Row 0: Read latency ---
    _hist_cdf(axes[0, 0], axes[0, 1], r_lats, "steelblue",
              f"reads (n={len(reads)})")
    axes[0, 0].set_title("Read Latency — histogram (log Y)")
    axes[0, 1].set_title("Read Latency — CDF (long tail)")

    # --- Row 1: Write latency ---
    _hist_cdf(axes[1, 0], axes[1, 1], w_lats, "tomato",
              f"writes (n={len(writes)})")
    axes[1, 0].set_title("Write Latency — histogram (log Y)")
    axes[1, 1].set_title("Write Latency — CDF (long tail)")

    # --- Row 2: Write->read gap + stale summary ---
    _gap_hist(axes[2, 0], gaps,
              f"Write→Read gap  (paired reads, n={len(gaps)})")

    # Text summary panel
    ax_txt = axes[2, 1]
    ax_txt.axis("off")
    stale_pct = 100 * stale / max(len(reads), 1)

    def _pct(lats, p):
        if not lats:
            return 0.0
        s = sorted(lats)
        return s[min(int(len(s) * p / 100), len(s) - 1)]

    summary = (
        f"SUMMARY\n"
        f"{'─'*32}\n"
        f"Total requests : {len(records)}\n"
        f"  Writes       : {len(writes)}\n"
        f"  Reads        : {len(reads)}\n\n"
        f"Write latency (ms)\n"
        f"  avg  : {sum(w_lats)/max(len(w_lats),1):.1f}\n"
        f"  p50  : {_pct(w_lats,50):.1f}\n"
        f"  p95  : {_pct(w_lats,95):.1f}\n"
        f"  p99  : {_pct(w_lats,99):.1f}\n\n"
        f"Read latency (ms)\n"
        f"  avg  : {sum(r_lats)/max(len(r_lats),1):.1f}\n"
        f"  p50  : {_pct(r_lats,50):.1f}\n"
        f"  p95  : {_pct(r_lats,95):.1f}\n"
        f"  p99  : {_pct(r_lats,99):.1f}\n\n"
        f"Stale reads    : {stale}/{len(reads)} ({stale_pct:.1f}%)\n"
        f"Paired gaps    : n={len(gaps)}"
        + (f"  avg={sum(gaps)/len(gaps):.0f}ms" if gaps else "")
    )
    ax_txt.text(0.05, 0.95, summary, transform=ax_txt.transAxes,
                fontsize=9, verticalalignment="top", fontfamily="monospace",
                bbox=dict(boxstyle="round", facecolor="lightyellow", alpha=0.8))

    plt.tight_layout()
    out_dir.mkdir(parents=True, exist_ok=True)
    out_path = out_dir / f"{stem}.png"
    plt.savefig(out_path, dpi=150)
    plt.close(fig)
    print(f"  Saved -> {out_path}")


# ---------------------------------------------------------------------------
# Summary figures: one per ratio, overlaying all configs
# ---------------------------------------------------------------------------

def plot_summary_by_ratio(all_data, out_dir):
    """
    all_data: { stem -> records }
    Group by ratio (w1r99, w10r90, w50r50, w90r10) and overlay configs.
    """
    # Extract ratio tag from stem: "w5r1_w1r99" -> "w1r99"
    by_ratio = defaultdict(dict)
    for stem, records in all_data.items():
        parts = stem.split("_", 1)
        if len(parts) == 2:
            config, ratio = parts
        else:
            config, ratio = stem, "unknown"
        by_ratio[ratio][config] = records

    colors = plt.cm.tab10.colors

    for ratio, config_data in sorted(by_ratio.items()):
        fig, axes = plt.subplots(1, 3, figsize=(16, 5))
        fig.suptitle(f"All Configs — ratio {ratio.upper()}", fontsize=13,
                     fontweight="bold")

        for idx, (config, records) in enumerate(sorted(config_data.items())):
            color = colors[idx % len(colors)]
            reads  = [r for r in records if r["op_type"] == "read"]
            writes = [r for r in records if r["op_type"] == "write"]
            gaps   = [r["write_read_gap_ms"] for r in reads
                      if r["write_read_gap_ms"] is not None]

            r_lats = sorted(r["latency_ms"] for r in reads)
            w_lats = sorted(r["latency_ms"] for r in writes)

            # Read CDF
            if r_lats:
                n = len(r_lats)
                axes[0].plot(r_lats, [(i+1)/n for i in range(n)],
                             color=color, label=config, linewidth=1.6)
            # Write CDF
            if w_lats:
                n = len(w_lats)
                axes[1].plot(w_lats, [(i+1)/n for i in range(n)],
                             color=color, label=config, linewidth=1.6)
            # Gap CDF
            if gaps:
                g = sorted(gaps)
                n = len(g)
                axes[2].plot(g, [(i+1)/n for i in range(n)],
                             color=color, label=config, linewidth=1.6)

        for ax, ttl in zip(axes, ["Read Latency CDF", "Write Latency CDF",
                                   "Write→Read Gap CDF"]):
            ax.set_title(ttl)
            ax.set_xlabel("ms")
            ax.set_ylabel("Cumulative fraction")
            ax.set_ylim(0, 1.05)
            ax.grid(True, linestyle="--", alpha=0.4)
            ax.legend(fontsize=8)

        plt.tight_layout()
        out_path = out_dir / f"summary_{ratio}.png"
        plt.savefig(out_path, dpi=150)
        plt.close(fig)
        print(f"  Summary saved -> {out_path}")


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def parse_args():
    p = argparse.ArgumentParser(description="Plot load test results")
    p.add_argument("input", help="CSV file or directory")
    p.add_argument("--out-dir", default=None)
    return p.parse_args()


if __name__ == "__main__":
    args = parse_args()
    input_path = Path(args.input)

    if input_path.is_file():
        csv_files = [input_path]
        out_dir = Path(args.out_dir) if args.out_dir else input_path.parent
    elif input_path.is_dir():
        csv_files = sorted(input_path.glob("*.csv"))
        out_dir = Path(args.out_dir) if args.out_dir else input_path
    else:
        print(f"Error: {input_path} not found", file=sys.stderr)
        sys.exit(1)

    if not csv_files:
        print("No CSV files found.", file=sys.stderr)
        sys.exit(1)

    all_data = {}
    for csv_path in csv_files:
        print(f"Plotting {csv_path.name} ...")
        plot_file(csv_path, out_dir)
        all_data[csv_path.stem] = load_csv(csv_path)

    if len(all_data) > 1:
        print("\nGenerating summary figures by ratio ...")
        plot_summary_by_ratio(all_data, out_dir)

    print("\nDone.")
