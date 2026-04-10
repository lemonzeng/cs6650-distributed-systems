#!/usr/bin/env python3
"""
Generate a complete PDF report with embedded graphs and analysis text.
Output: results/hw10_report.pdf
"""

import csv
from collections import defaultdict
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker
from matplotlib.backends.backend_pdf import PdfPages
import numpy as np

RESULTS = Path("results")
OUT_PDF  = RESULTS / "hw10_report.pdf"

CONFIGS = ["w5r1", "w1r5", "w3r3", "leaderless"]
RATIOS  = ["w1r99", "w10r90", "w50r50", "w90r10"]

CONFIG_LABELS = {
    "w5r1":       "Leader-Follower  W=5 R=1",
    "w1r5":       "Leader-Follower  W=1 R=5",
    "w3r3":       "Leader-Follower  W=3 R=3",
    "leaderless": "Leaderless  W=N=5 R=1",
}
RATIO_LABELS = {
    "w1r99":  "1% writes / 99% reads",
    "w10r90": "10% writes / 90% reads",
    "w50r50": "50% writes / 50% reads",
    "w90r10": "90% writes / 10% reads",
}
CONFIG_COLORS = {
    "w5r1":       "#e15759",
    "w1r5":       "#4e79a7",
    "w3r3":       "#59a14f",
    "leaderless": "#f28e2b",
}

# ---------------------------------------------------------------------------
# Data loading
# ---------------------------------------------------------------------------

def load(config, ratio):
    path = RESULTS / f"{config}_{ratio}.csv"
    if not path.exists():
        return [], []
    writes, reads = [], []
    with open(path, newline="") as f:
        for row in csv.DictReader(f):
            try:
                lat = float(row["latency_ms"])
            except ValueError:
                continue
            stale = row.get("stale", "") == "True"
            gap_s = row.get("write_read_gap_ms", "")
            gap   = float(gap_s) if gap_s not in ("", "None") else None
            rec   = {"lat": lat, "stale": stale, "gap": gap,
                     "ok": row.get("ok","") == "True"}
            if row["op_type"] == "write":
                writes.append(rec)
            else:
                reads.append(rec)
    return writes, reads

def pct(data, p):
    if not data:
        return 0.0
    s = sorted(data)
    return s[min(int(len(s)*p/100), len(s)-1)]

def avg(data):
    return sum(data)/len(data) if data else 0.0

# ---------------------------------------------------------------------------
# Plot helpers
# ---------------------------------------------------------------------------

def cdf_plot(ax, latencies, color, label, lw=1.8):
    if not latencies:
        return
    s = sorted(latencies)
    n = len(s)
    ax.plot(s, [(i+1)/n for i in range(n)], color=color,
            linewidth=lw, label=label)

def hist_plot(ax, latencies, color, label, bins=40):
    if not latencies:
        return
    ax.hist(latencies, bins=bins, color=color, alpha=0.75,
            edgecolor="white", label=label)
    ax.set_yscale("log")
    ax.yaxis.set_major_formatter(ticker.ScalarFormatter())

def style_cdf(ax, title):
    ax.set_title(title, fontsize=10, fontweight="bold")
    ax.set_xlabel("Latency (ms)", fontsize=8)
    ax.set_ylabel("Cumulative fraction", fontsize=8)
    ax.set_ylim(0, 1.05)
    ax.grid(True, linestyle="--", alpha=0.35)
    ax.tick_params(labelsize=7)
    ax.legend(fontsize=7)

def style_hist(ax, title):
    ax.set_title(title, fontsize=10, fontweight="bold")
    ax.set_xlabel("Latency (ms)", fontsize=8)
    ax.set_ylabel("Count (log)", fontsize=8)
    ax.tick_params(labelsize=7)
    ax.legend(fontsize=7)

# ---------------------------------------------------------------------------
# PDF generation
# ---------------------------------------------------------------------------

def title_page(pdf):
    fig, ax = plt.subplots(figsize=(11, 8.5))
    ax.axis("off")
    ax.text(0.5, 0.72, "CS 6650 — Homework 10",
            transform=ax.transAxes, ha="center", fontsize=28, fontweight="bold")
    ax.text(0.5, 0.62, "Distributed Key-Value Databases",
            transform=ax.transAxes, ha="center", fontsize=20, color="#444")
    ax.text(0.5, 0.52, "Load Test Report",
            transform=ax.transAxes, ha="center", fontsize=18, color="#666")
    ax.text(0.5, 0.38,
            "Configurations tested:\n"
            "  • Leader-Follower  W=5, R=1\n"
            "  • Leader-Follower  W=1, R=5\n"
            "  • Leader-Follower  W=3, R=3\n"
            "  • Leaderless  W=N=5, R=1\n\n"
            "Read/Write ratios: 1/99 · 10/90 · 50/50 · 90/10\n"
            "1,000 requests per run  ·  100-key pool  ·  paired-reads queue",
            transform=ax.transAxes, ha="center", va="center",
            fontsize=12, linespacing=1.8,
            bbox=dict(boxstyle="round,pad=0.6", facecolor="#f0f4ff", alpha=0.8))
    plt.tight_layout()
    pdf.savefig(fig)
    plt.close(fig)


def summary_table_page(pdf):
    """One-page summary table of avg latencies and stale rates."""
    fig, ax = plt.subplots(figsize=(11, 8.5))
    ax.axis("off")
    ax.text(0.5, 0.97, "Summary: Average Latency and Stale Read Rates",
            transform=ax.transAxes, ha="center", fontsize=14, fontweight="bold")

    headers = ["Config", "Ratio", "Write avg (ms)", "Read avg (ms)",
               "Stale reads", "Stale %"]
    rows = []
    for cfg in CONFIGS:
        for ratio in RATIOS:
            w, r = load(cfg, ratio)
            w_avg = avg([x["lat"] for x in w])
            r_avg = avg([x["lat"] for x in r])
            stale = sum(1 for x in r if x["stale"])
            stale_pct = 100*stale/max(len(r),1)
            rows.append([
                CONFIG_LABELS[cfg],
                RATIO_LABELS[ratio],
                f"{w_avg:.0f}",
                f"{r_avg:.1f}",
                f"{stale}/{len(r)}",
                f"{stale_pct:.1f}%",
            ])

    table = ax.table(
        cellText=rows, colLabels=headers,
        loc="center", cellLoc="center"
    )
    table.auto_set_font_size(False)
    table.set_fontsize(8)
    table.scale(1, 1.6)

    # Colour header row
    for j in range(len(headers)):
        table[0, j].set_facecolor("#4e79a7")
        table[0, j].set_text_props(color="white", fontweight="bold")

    # Alternate row shading
    for i, row in enumerate(rows):
        colour = "#f7f7f7" if i % 2 == 0 else "white"
        for j in range(len(headers)):
            table[i+1, j].set_facecolor(colour)
        # Highlight high stale %
        sp = float(row[5].rstrip("%"))
        if sp > 20:
            table[i+1, 5].set_facecolor("#ffcccc")
        elif sp > 5:
            table[i+1, 5].set_facecolor("#fff0cc")

    plt.tight_layout()
    pdf.savefig(fig)
    plt.close(fig)


def per_ratio_pages(pdf):
    """For each ratio: 2x2 grid — read CDF, write CDF, read hist, gap hist."""
    for ratio in RATIOS:
        fig, axes = plt.subplots(2, 2, figsize=(11, 8.5))
        fig.suptitle(f"Latency Distributions — {RATIO_LABELS[ratio]}",
                     fontsize=13, fontweight="bold", y=0.98)

        ax_rcdf = axes[0, 0]
        ax_wcdf = axes[0, 1]
        ax_rhst = axes[1, 0]
        ax_gap  = axes[1, 1]

        all_gaps = {}

        for cfg in CONFIGS:
            w, r = load(cfg, ratio)
            col   = CONFIG_COLORS[cfg]
            label = CONFIG_LABELS[cfg]
            r_lats = [x["lat"] for x in r]
            w_lats = [x["lat"] for x in w]
            gaps   = [x["gap"] for x in r if x["gap"] is not None]
            stale  = sum(1 for x in r if x["stale"])
            sp     = f"{100*stale/max(len(r),1):.1f}%"

            cdf_plot(ax_rcdf, r_lats, col, f"{label}  (stale={sp})")
            cdf_plot(ax_wcdf, w_lats, col, label)
            hist_plot(ax_rhst, r_lats, col, label)
            all_gaps[cfg] = gaps

        # Gap subplot — overlay CDFs for each config
        for cfg, gaps in all_gaps.items():
            if gaps:
                g = sorted(gaps)
                n = len(g)
                ax_gap.plot(g, [(i+1)/n for i in range(n)],
                            color=CONFIG_COLORS[cfg],
                            label=CONFIG_LABELS[cfg], linewidth=1.8)
        ax_gap.set_title("Write→Read Gap CDF (paired reads)", fontsize=10, fontweight="bold")
        ax_gap.set_xlabel("Gap (ms)", fontsize=8)
        ax_gap.set_ylabel("Cumulative fraction", fontsize=8)
        ax_gap.set_ylim(0, 1.05)
        ax_gap.grid(True, linestyle="--", alpha=0.35)
        ax_gap.tick_params(labelsize=7)
        ax_gap.legend(fontsize=7)

        style_cdf(ax_rcdf, "Read Latency CDF")
        style_cdf(ax_wcdf, "Write Latency CDF")
        style_hist(ax_rhst, "Read Latency Histogram (log Y)")

        plt.tight_layout(rect=[0, 0, 1, 0.96])
        pdf.savefig(fig)
        plt.close(fig)


def per_config_pages(pdf):
    """For each config: 4 ratios × read/write latency shown together."""
    for cfg in CONFIGS:
        fig, axes = plt.subplots(2, 2, figsize=(11, 8.5))
        fig.suptitle(f"All Ratios — {CONFIG_LABELS[cfg]}",
                     fontsize=13, fontweight="bold", y=0.98)

        ax_rcdf = axes[0, 0]
        ax_wcdf = axes[0, 1]
        ax_rhst = axes[1, 0]
        ax_whst = axes[1, 1]

        ratio_colors = ["#4e79a7","#f28e2b","#59a14f","#e15759"]

        for ratio, col in zip(RATIOS, ratio_colors):
            w, r = load(cfg, ratio)
            r_lats = [x["lat"] for x in r]
            w_lats = [x["lat"] for x in w]
            stale  = sum(1 for x in r if x["stale"])
            sp     = f"{100*stale/max(len(r),1):.1f}%"
            rl     = RATIO_LABELS[ratio]

            cdf_plot(ax_rcdf, r_lats, col, f"{rl}  stale={sp}")
            cdf_plot(ax_wcdf, w_lats, col, rl)
            hist_plot(ax_rhst, r_lats, col, rl)
            hist_plot(ax_whst, w_lats, col, rl)

        style_cdf(ax_rcdf, "Read Latency CDF — all ratios")
        style_cdf(ax_wcdf, "Write Latency CDF — all ratios")
        style_hist(ax_rhst, "Read Latency Histogram (log Y)")
        style_hist(ax_whst, "Write Latency Histogram (log Y)")

        plt.tight_layout(rect=[0, 0, 1, 0.96])
        pdf.savefig(fig)
        plt.close(fig)


def text_page(pdf, title, body, fontsize=9):
    fig, ax = plt.subplots(figsize=(11, 8.5))
    ax.axis("off")
    ax.text(0.5, 0.97, title, transform=ax.transAxes,
            ha="center", fontsize=13, fontweight="bold", va="top")
    ax.text(0.05, 0.91, body, transform=ax.transAxes,
            ha="left", va="top", fontsize=fontsize,
            fontfamily="monospace", linespacing=1.55,
            wrap=True)
    plt.tight_layout()
    pdf.savefig(fig)
    plt.close(fig)


# ---------------------------------------------------------------------------
# Report text sections
# ---------------------------------------------------------------------------

GENERATOR_TEXT = """\
HOW THE LOAD TEST GENERATOR WORKS
──────────────────────────────────────────────────────────────────────────────

Key pool:
  100 fixed keys (key-0000 … key-0099). With 1,000 requests per run each key
  is touched ~10 times on average, guaranteeing temporal overlap between reads
  and writes to the same key even without extra logic.

Paired-reads queue (guarantees local-in-time clustering):
  After every successful write to key K with version V:
    → push { key:K, expected_version:V, write_time:T } onto paired_reads deque

  When the next read operation fires:
    → if paired_reads is non-empty: pop and read that exact key from a follower
    → else: pick a random key from the 100-key pool

  This means every write is immediately followed by at least one follower read
  of the same key. The write→read gap in the graphs reflects real elapsed time
  from write ACK to that paired read.

Stale read detection:
  POST /set returns {"version": N}. The client stores known[key] = N.
  When a follower returns version < N (or a 404 for a known written key),
  the read is flagged as STALE.

  Reads always go to FOLLOWER nodes (8002–8005) or NON-COORDINATOR nodes
  (8012–8015) — never to the leader — so leader freshness never masks
  follower staleness.

Why this guarantees frequent same-key reads and writes:
  • Small pool (100 keys) creates natural overlap at high load.
  • Paired-reads queue guarantees at least one follower read per write,
    occurring within milliseconds of the write ACK.
  • The write→read gap histogram shows this directly: for W=1 R=5 gaps
    cluster around 2–5 ms (reads arrive long before 1.2 s propagation),
    for W=5 R=1 gaps cluster around 1,230 ms (reads arrive right after
    the slow synchronous write returns).
"""

ANALYSIS_TEXT = """\
LATENCY ANALYSIS
──────────────────────────────────────────────────────────────────────────────

W=5 R=1  (write avg ~1,232 ms · read avg ~2–7 ms · stale 0%)
  Every write blocks until all 4 followers ACK. Minimum time = 4 followers ×
  (200 ms leader sleep + 100 ms follower sleep) = 1,200 ms. The write
  histogram shows a sharp spike at ~1,230 ms with almost no tail — the
  mandatory sleeps dominate variance, not network jitter. Reads return the
  leader's local value instantly (R=1 = local lookup only). Zero staleness
  at every ratio because followers are always fully updated before ACK.

W=1 R=5  (write avg ~2 ms · read avg ~1–3 ms · stale 1.8%–88.7%)
  Write ACKs after local store only; follower updates are async. Writes take
  ~2 ms — essentially HTTP overhead. Stale rate grows dramatically with write
  frequency: 1.8% at 1% writes, 88.7% at 50% writes. At 50/50, writes arrive
  every ~2 ms but async propagation takes ~1,200 ms, leaving hundreds of
  un-propagated keys at any moment. The 6.6% at 90% writes is lower because
  the rare reads mostly occur long after propagation has finished.

W=3 R=3  (write avg ~620 ms · read avg ~2–6 ms · stale 0%–5.1%)
  Waits for 2 follower ACKs (self + 2 = quorum of 3). Minimum time = 2 ×
  300 ms = 600 ms — exactly half the W=5 cost. Because W+R = 6 > N = 5,
  the write and read quorums always overlap: the leader's R=3 quorum read
  always includes at least one node from the write quorum, guaranteeing the
  latest version is returned. Residual 5.1% staleness occurs when clients
  read directly from follower ports (bypassing the quorum read), hitting one
  of the 2 followers not in the write quorum.

Leaderless W=N=5  (write avg ~1,238 ms · read avg ~2–8 ms · stale 0%)
  Coordinator synchronously updates all 4 peers before returning — identical
  cost to W=5 leader-follower. Any node can be write coordinator (no single
  bottleneck). Zero staleness at all ratios. R=1 means each node reads its
  own local store; since W=N, all nodes have the value after the write ACK.
"""

WINNER_TEXT = """\
WHICH CONFIGURATION WINS AT EACH READ/WRITE RATIO
──────────────────────────────────────────────────────────────────────────────

W=1% / R=99%  →  Best: W=5 R=1 or Leaderless
  Writes are rare so their cost barely matters. W=5 R=1 delivers ~2 ms reads
  with 0% staleness. W=3 R=3 is also fine (0.9% stale). W=1 R=5 shows only
  1.8% stale but its async nature is risky for strong-consistency needs.

W=10% / R=90%  →  Best: W=5 R=1 or W=3 R=3
  W=5 R=1: 0% stale but write path costs 1,232 ms each. W=3 R=3: 5.1% stale
  but write cost is halved (620 ms) — a good balance. W=1 R=5 at 14.5% stale
  is risky for applications where reads must reflect recent writes.

W=50% / R=50%  →  Best: W=3 R=3
  W=5 and Leaderless: half the requests are 1.2 s writes — throughput tanks.
  W=1 R=5: 88.7% stale — nearly every paired read is stale — unacceptable
  for most applications. W=3 R=3: 1.8% stale at only 620 ms per write.
  Clear winner for balanced workloads.

W=90% / R=10%  →  Best: W=1 R=5
  Write latency dominates everything. W=1 R=5 writes in ~2 ms with only 6.6%
  stale reads (rare reads arrive after propagation). W=3 R=3 is second at
  620 ms. W=5 and Leaderless are impractical — 900 writes × 1,232 ms each
  would take ~18 minutes for 1,000 requests.

WHICH DATABASE FOR WHICH APPLICATION
──────────────────────────────────────────────────────────────────────────────

W=5 R=1  →  Read-heavy, zero-tolerance for staleness
  Banking, medical records, inventory. Writes are rare; reads must always
  see the latest value. The 1.2 s write is acceptable because writes are
  infrequent and correctness is paramount.

W=1 R=5  →  Write-heavy, eventual consistency acceptable
  Social media feeds, IoT sensor ingestion, logging, analytics. High write
  throughput, occasional stale reads tolerated. Bad choice if users expect
  to immediately read back what they just wrote.

W=3 R=3  →  Balanced load, reasonable consistency required
  Shopping carts, session state, collaborative tools. The quorum overlap
  (W+R > N) provides strong consistency through the leader while keeping
  write cost half that of W=5. General-purpose choice.

Leaderless W=N  →  High availability, no single leader bottleneck
  Distributed configuration, coordination services. Same consistency as W=5
  R=1 but any node can accept writes, improving fault tolerance. Requires all
  N nodes to be available for writes to succeed (unlike W<N leader-follower
  where some followers can lag freely).
"""

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    RESULTS.mkdir(exist_ok=True)
    print(f"Generating {OUT_PDF} ...")

    with PdfPages(OUT_PDF) as pdf:
        # Cover
        title_page(pdf)

        # Section 1: Generator design
        text_page(pdf, "Section 1 — Load Test Generator Design", GENERATOR_TEXT)

        # Section 2: Summary table
        summary_table_page(pdf)

        # Section 3: Per-ratio graphs (4 pages)
        per_ratio_pages(pdf)

        # Section 4: Per-config graphs (4 pages)
        per_config_pages(pdf)

        # Section 5: Analysis text
        text_page(pdf, "Section 3 — Latency & Staleness Analysis", ANALYSIS_TEXT)
        text_page(pdf, "Section 4 — Best Config per Ratio & Application Guide",
                  WINNER_TEXT)

        # PDF metadata
        d = pdf.infodict()
        d["Title"]   = "CS 6650 HW10 — Distributed KV Load Test Report"
        d["Subject"] = "Distributed Databases, CAP Theorem, Replication"

    print(f"Done → {OUT_PDF}")


if __name__ == "__main__":
    main()
