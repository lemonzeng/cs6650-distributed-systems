# HW10 展示脚本 — 中文版

---

## 第一部分：实验概览——目的与代码结构

### 开场

> "这个作业的核心是 CAP 定理的动手实现。我们构建了两种分布式 Key-Value 存储服务——主从架构（Leader-Follower）和无主架构（Leaderless）——并在四种不同的读写比例下进行压测，直观观察一致性和延迟之间如何权衡。"

### 我们构建了什么

**两个服务，对外暴露相同的两个 API：**
- `POST /set` — 写入键值对，返回 `{"version": N}` 和 201-Created
- `GET /get?key=K` — 读取值，返回 200 或 404

**主从架构（`leader-follower/`）：**
- 1 个 Leader + 4 个 Follower，共 N=5 个节点
- 三种配置：W=5 R=1 / W=1 R=5 / W=3 R=3
- 通过 `docker-compose.yml` 及覆盖文件部署

**无主架构（`leaderless/`）：**
- 5 个平等节点，任意节点都可以成为写协调者
- 固定为 W=N=5，R=1
- 通过 `docker-compose.leaderless.yml` 部署

**压力测试（`load-tester/`）：**
- Python 脚本，按指定读写比例生成请求
- 记录每个请求的延迟，通过版本号比较检测脏读
- 输出 CSV 结果和四格 PNG 图表

**单元测试（`tests/`）：**
- pytest 测试，验证每种配置下的一致性行为

### 为什么要设置人工延迟？

作业要求：
- Leader 每次向 Follower 推送后睡眠 **200 ms**
- Follower 收到更新后睡眠 **100 ms**
- Follower 处理内部读请求时睡眠 **50 ms**

这些延迟将一致性窗口从微秒级扩大到秒级，让脏读在压测中可以被清晰地测量和观察到。

---

## 第二部分：关键代码讲解

### 2.1 写路径 — `leaderSet`（主从架构）

**文件：** `leader-follower/main.go`

```
第 145 行 — func leaderSet(...)
第 152 行 — version := atomic.AddInt64(&versionCounter, 1)
第 153 行 — localWrite(req.Key, req.Value, version)
第 155 行 — acks := 1  // 自己算第 1 个 ack
```

Leader 立刻写入本地存储，并将自己计为第 1 个 ack。然后：

```
第 158 行 — if wQuorum == 1 {
第 160 行 —     remaining = followerURLs   // W=1：所有 Follower 都异步推送
            } else {
第 163 行 —     for _, furl := range followerURLs {
第 164 行 —         if acks >= wQuorum { remaining = append(...); continue }
第 168 行 —         err := postUpdate(furl, ...)   // 同步推送给 Follower
第 169 行 —         time.Sleep(200 * time.Millisecond)   // Leader 200ms 睡眠
第 171 行 —         if err == nil { acks++ }
            }
```

**核心逻辑：**
- W=1：完全跳过同步推送，所有 Follower 都异步
- W=3：逐个联系 Follower，直到凑够 3 个 ack（自己 + 2 个），剩余的异步
- W=5：依次同步联系全部 4 个 Follower

```
第 177 行 — for _, furl := range remaining {
第 178 —     go func(u string) { postUpdate(u, ...) }(furl)   // 后台 goroutine 异步推送
```

不在写 quorum 内的 Follower，通过后台 goroutine 异步更新。这是 W=1 产生一致性窗口、W=3 有残余脏读的根本原因。

**延迟推算：**
- W=5：4 个 Follower × (200 + 100) ms = **最小 1,200 ms**
- W=3：2 个 Follower × 300 ms = **最小 600 ms**
- W=1：0 个同步跳转 = **约 2 ms**（本地写 + HTTP 响应）

---

### 2.2 读路径 — `leaderGet`

**文件：** `leader-follower/main.go`

```
第 186 行 — func leaderGet(...)
第 193 行 — local, ok := localRead(key)

第 195 行 — if rQuorum == 1 {
第 200 行 —     writeJSON(w, http.StatusOK, local)   // R=1：直接返回本地值，无网络跳转
第 201 行 —     return
```

R=1 是纯本地查询——无任何网络开销，这就是 W=5 R=1 读延迟只有 ~2ms 的原因。

当 R > 1 时：

```
第 209 行 — needed := rQuorum - 1   // 并行联系 R-1 个 Follower
第 214 行 — for i := 0; i < needed; i++ {
第 215 —     go func(u string) {
第 216 —         e, err := getInternalRead(u, key)   // 调用 /internal/read，有 50ms 睡眠
第 220 —         ch <- result{entry: e, ok: true}
```

```
第 225 行 — best := local
第 229 行 — if res.entry.Version > best.Version {
第 230 —     best = res.entry    // 取所有节点中版本号最大的
```

**Quorum 重叠保证：** W=3 R=3 时，W+R = 6 > N = 5，写集合和读集合必然至少有一个节点重叠。那个重叠节点一定有最新的值，所以 `best` 一定是当前最新版本。

**重要说明：** 压测代码直接读 Follower 的 `/get` 端口，完全绕过了 `leaderGet`。这是故意设计的——目的是测量 Follower 的"原始本地状态"，而非经过 Quorum 协调后的视图。

---

### 2.3 Follower 的三个端点——行为截然不同

**文件：** `leader-follower/main.go`

```
第 246 行 — func followerInternalUpdate(...)   // Leader 写传播时调用
第 252 行 —     time.Sleep(100 * time.Millisecond)   // 100ms 模拟存储写入
第 253 行 —     localWrite(req.Key, req.Value, req.Version)
```

```
第 257 行 — func followerInternalRead(...)   // Leader Quorum 读时调用（R > 1）
第 263 行 —     time.Sleep(50 * time.Millisecond)   // 50ms 模拟存储读取
第 264 行 —     e, ok := localRead(key)
```

```
第 273 行 — func followerGet(...)   // 客户端直接读 —— 无睡眠，无协调
第 280 行 —     e, ok := localRead(key)
第 284 行 —     writeJSON(w, http.StatusOK, e)
```

三个端点，三种行为：
| 端点 | 谁调用 | 有无延迟 | 用途 |
|------|--------|---------|------|
| `/internal/update` | Leader 写传播 | 100ms | 模拟存储写入 |
| `/internal/read` | Leader Quorum 读 | 50ms | 模拟存储读取 |
| `/get` | 客户端直接读 | **无** | 暴露 Follower 真实本地状态 |

**这就是为什么压测的读延迟是 1–3ms 而非 50ms**：压测走的是 `/get`（无睡眠），不是 `/internal/read`（50ms 睡眠）。

---

### 2.4 无主架构写路径 — `handleSet`

**文件：** `leaderless/main.go`

```
第 118 行 — func handleSet(...)
第 125 行 —     version := time.Now().UnixNano()   // 纳秒时间戳作版本号，无需中央计数器
第 126 行 —     localWrite(req.Key, req.Value, version)

第 129 行 —     for _, purl := range peerURLs {
第 130 行 —         postUpdate(purl, req.Key, req.Value, version)
第 131 行 —         time.Sleep(200 * time.Millisecond)   // 同样的 200ms 延迟
```

关键架构区别：无主架构用 `time.Now().UnixNano()` 作版本号，而非中央原子计数器。这意味着任何节点都可以独立生成版本号，不需要固定的 Leader 来序列化写操作。

**Last-Write-Wins 冲突解决（`localWrite`）：**

```
第 80 行 — func localWrite(key, value string, version int64) {
第 84 行 —     if !ok || version > cur.Version {   // 只在版本号更大时才更新
第 85 行 —         store[key] = Entry{Value: value, Version: version}
```

如果两个并发写因网络延迟乱序到达，节点始终保留时间戳更大的那个值，防止旧数据覆盖新数据。

---

### 2.5 压测代码 — 配对读与脏读检测

**文件：** `load-tester/load_test.py`

**Key 池：**
```
第 96 行 — KEY_POOL = [f"key-{i:04d}" for i in range(100)]
```
100 个 key，1,000 个请求，每个 key 平均被访问 10 次，天然产生时间上的读写重叠。

**写操作与版本追踪：**
```
第 109 行 — r = requests.post(f"{write_url}/set", json={"key": key, "value": value}, ...)
第 116 行 — if r.status_code == 201:
第 122 行 —     known[key] = version   # 记住服务端返回的版本号
```

**配对读队列——实现"时间局部性"的核心机制：**
```
第 232 行 — if rec["ok"] and key in known:
第 233 —     paired_reads.append({
第 235 —         "expected_version": known[key],
第 236 —         "write_time":       time.time(),   # 写 ACK 返回的时刻
```
```
第 242 行 — if paired_reads:
第 244 —     item = paired_reads.popleft()   # 取出刚才写的同一个 key
第 245 —     rec = do_read(read_url, item["key"], item["expected_version"], item["write_time"])
```

每次写操作完成后，将该 key 压入 `paired_reads` 队列。下一次读操作优先弹出队列，保证读的是同一个 key 且是写操作刚刚完成后的瞬间。

**写→读时间间隔：**
```
第 150 行 — gap_ms = round((time.time() - write_time) * 1000, 3)
```
即"写 ACK 返回"到"配对读开始"的时间差。W=1 时约 2ms；W=5 时约 1,230ms。

**脏读检测：**
```
第 159 行 — if r.status_code == 200 and expected_version is not None:
第 161 —     returned_version = r.json().get("version", -1)
第 162 —     if returned_version < expected_version:
第 163 —         stale = True
第 166 — elif r.status_code == 404 and expected_version is not None:
第 168 —     stale = True   # key 已写入但 Follower 返回 404 → 脏读
```

---


## 第三部分：逐节讲解 Report

> 现在打开 `report.md`，按 Section 顺序讲解。

---

### → Section 1：Load Test Design

> "在看任何数字之前，先说清楚数据是怎么产生的。"

指向两个段落：

- **Paired-reads queue（配对读队列）**：每次写操作完成后，把这个 key 压入队列；下一次读操作优先从队列中取这个 key 来读。这保证了读和写以极短的时间间隔作用于同一个 key——脏读才能被稳定、持续地观测到。
- **脏读检测**：服务端每次写成功都返回版本号，我们存入 `known[key]`。如果 Follower 返回的版本号更低（或者返回 404），就记为脏读。读永远打到 Follower 端口，不经过 Leader，所以 Leader 的 Quorum 读逻辑不会掩盖任何不一致。

---

### → Section 2：Delay Model

> "在看结果之前，先建立一个数学预期，这样你看到图的时候不会觉得数字是随机的。"

指向**第二张表**（Write latency floor）：

> "所有写延迟数字都是这个公式直接算出来的。W=5 同步联系 4 个 Follower：4 × 300ms = 1200ms 最小值。W=3 联系 2 个：600ms。W=1 一个都不联系：约 2ms。这是硬下限，不是平均值。所以你在图里看到的写延迟直方图都是一个极尖锐的峰，几乎没有尾巴——延迟完全由 `time.Sleep()` 决定，没有任何随机性。"

---

### → Section 3：Summary Table

> "这张表是整个实验的核心结果。Section 4 的图是在解释为什么这张表长这个样子。"

滚动浏览表格，重点讲三件事：

**1. 写延迟列——竖着扫一遍。**
> "W=5 和 Leaderless 始终在 ~1230ms，W=3 始终在 ~616ms，W=1 始终在 ~2ms。读写比例对写延迟没有任何影响。从 1% 写到 90% 写，每次写的成本完全不变——比例只决定你多久付一次这个成本。"

**2. 读延迟列——注意轻微上升。**
> "W=5 的读延迟从 1% 写时的 2.0ms 上升到 90% 写时的 7.1ms。这不是一致性问题，是锁竞争——写压力越大，in-memory store 的读写锁竞争越激烈，读延迟略微上升。"

**3. Stale % 列——指向三个高亮单元格。**
> "这是最有趣的部分。看 W=1 R=5 这四行：1% 写时脏读 1.8%，10% 写时 14.5%，50/50 时跳到 88.7%（红色高亮），然后 90% 写时反而降回 6.6%。这个非单调的模式是整个实验的关键洞察：决定脏读率的不是写了多少次，而是配对读是否在 ~1200ms 的传播窗口内到达。50/50 时写每隔 ~2ms 就来一次，传播永远追不上，几乎每次配对读都命中脏 Follower。90% 写时读极少，大概率命中的是传播早已完成的旧 key。"

---

### → Section 4：Results by Read/Write Ratio

> "现在看图。每张 summary 图把四种配置的延迟 CDF 叠在一起，这个视角比单独看每个配置更直观。"

**W=1% R=99% 的图：**
> "99% 是读，写只在 1000 个请求里出现 10 次。W=5 和 Leaderless 完胜——0% 脏读，2ms 读延迟。写的 1200ms 成本在这个比例下几乎感知不到，整体吞吐不受影响。"

**W=10% R=90% 的图：**
> "现在写占 10%，W=5 要付 100 次 1200ms 的代价。W=3 R=3 把这个成本砍到一半（600ms），脏读只有 5.1%。W=1 R=5 的 14.5% 脏读已经比较明显了，对很多应用来说不可接受。W=3 R=3 是这个比例下最务实的选择。"

**W=50% R=50% 的图：**
> "这张图最直观。W=5 和 Leaderless 的 CDF 在 ~1200ms 处有一个巨大的阶梯——一半的请求都要等超过一秒，吞吐量直接被砍半。W=1 R=5 很快，但 88.7% 脏读，对任何需要'写完能立刻读到'的应用来说几乎没有可用性。W=3 R=3 是唯一同时兼顾合理写延迟和低脏读率的选项，胜出毫无悬念。"

**W=90% R=10% 的图：**
> "90% 写的时候，写延迟就是系统吞吐量的全部。W=5 要花 900 × 1200ms 等写，W=1 只花 900 × 2ms。6.6% 的脏读在这里可以接受——读太少了，大多数落在传播窗口关闭之后。W=1 R=5 以 600 倍的优势胜出。"

---

### → Section 5：Application Mapping

> "最后这张表把结论落到实际应用场景。"

逐行指向：
- **W=5 R=1** → 银行、医疗：写很少，但绝对不能有脏读
- **W=1 R=5** → IoT 采集、社交动态：需要极高写入速度，短暂脏读可接受——但绝对不适合用户写完立刻读回自己数据的场景
- **W=3 R=3** → 购物车、Session、通用 API：W+R > N 保证 Quorum 重叠，Leader 读路径永远返回最新值，是最通用的选择
- **Leaderless** → 分布式配置、地理分布的系统：没有固定 Leader，任何节点都可以协调写，但代价是所有 N 个节点必须同时在线才能完成写操作

> "最终结论：没有免费的午餐。代码里的人工睡眠放大了数字，但权衡曲线的形状和 DynamoDB、Cassandra、Riak 在生产环境中完全一致。工程师选数据库时永远要先问两个问题：**你的读写比例是多少？你能接受多少脏读？**"
