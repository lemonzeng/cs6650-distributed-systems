代码里有几个真实的 race condition，两个数据库处理方式完全不同。                
   
  ---                                                                            
  Race 1：两个请求同时给同一购物车添加同一产品                                 
                                                                                 
  MySQL — 安全，数据库级别原子                                                   

  // mysql-service/main.go:210-214
  INSERT INTO cart_items (cart_id, product_id, quantity)
  VALUES (?, ?, ?)
  ON DUPLICATE KEY UPDATE quantity = quantity + VALUES(quantity)

  Thread A: INSERT product_id=5, quantity=2  ──┐
  Thread B: INSERT product_id=5, quantity=3  ──┤ 同时到达
                                                ↓
  InnoDB 行锁：一个先拿到锁，另一个等待
  Thread A 执行：INSERT → 成功，quantity=2
  Thread B 执行：UNIQUE 冲突 → UPDATE quantity = 2 + 3 = 5

  结果：一行，quantity=5  ✓ 正确

  InnoDB 的行锁 + ON DUPLICATE KEY UPDATE 保证两次写是串行的，最终结果正确。

  ---
  DynamoDB — 数据正确性有问题

  // dynamodb-service/main.go:179
  "SET #items = list_append(if_not_exists(#items, :empty), :new_item)"

  Thread A: list_append(items, [{product_id:5, quantity:2}])  ──┐
  Thread B: list_append(items, [{product_id:5, quantity:3}])  ──┘ 同时到达

  DynamoDB UpdateItem 是原子的，但两次都是追加操作：

  执行后：
  items = [
    {product_id: 5, quantity: 2},   ← Thread A 追加
    {product_id: 5, quantity: 3}    ← Thread B 追加
  ]

  结果：两行，quantity 没合并  ✗ 数据不正确

  list_append 不检查 List 内部有没有相同 product_id，并发只会追加更多重复条目。

  ---
  Race 2：删购物车的同时添加商品（TOCTOU）

  MySQL — 两步操作，但 FK 兜底

  // mysql-service/main.go:193-207
  // 第一步：检查购物车存在
  db.QueryRow("SELECT id FROM carts WHERE id = ?", id).Scan(&exists)

  // ← 这里有个 GAP，另一个请求可能在此时删掉了 cart

  // 第二步：插入商品
  db.Exec(`INSERT INTO cart_items (cart_id, ...) ...`)

  这是经典的 TOCTOU（Time of Check to Time of Use）问题。但 MySQL 有 FK 兜底：

  FOREIGN KEY (cart_id) REFERENCES carts(id) ON DELETE CASCADE

  Thread A: SELECT cart_id=1 → 存在
  Thread B: DELETE carts WHERE id=1 → 成功，CASCADE 删掉所有 items
  Thread A: INSERT cart_items (cart_id=1) → FK 约束失败，返回 DB_ERROR

  结果：没有孤儿数据，但 Thread A 返回 500 而不是 404  ✗ 语义不准确

  ---
  DynamoDB — 单次原子操作，语义正确

  // dynamodb-service/main.go:174-197
  ddbClient.UpdateItem(context.TODO(), &dynamodb.UpdateItemInput{
      ...
      ConditionExpression: aws.String("attribute_exists(cart_id)"),  // 原子检查
      ...
  })

  检查和更新在同一个原子操作里完成：

  Thread A: UpdateItem (检查 + 写入，原子)
  Thread B: DeleteItem cart_id=1

  如果 B 先执行：A 的 ConditionExpression 失败 → ConditionalCheckFailedException
  → 404  ✓
  如果 A 先执行：A 成功写入，B 再删除  ✓

  无论顺序如何，行为语义都正确

  ---
  Race 3：两个请求同时创建购物车

  MySQL — 安全

  result, err := db.Exec("INSERT INTO carts (customer_id) VALUES (?)",
  body.CustomerID)
  id, _ := result.LastInsertId()

  AUTO_INCREMENT 由数据库统一分配，并发插入不会冲突，每个请求得到唯一 ID。

  ---
  DynamoDB — 有碰撞风险

  // dynamodb-service/main.go:77
  cartID := strconv.FormatInt(time.Now().UnixNano(), 10)

  用 UnixNano 做 ID，两个请求如果在同一纳秒内执行（容器内并发完全可能）：

  Thread A: cartID = "1700000000000000000"
  Thread B: cartID = "1700000000000000000"  ← 相同！

  Thread A: PutItem → 写入 cart
  Thread B: PutItem → 静默覆盖 Thread A 的 cart（没有任何报错）

  结果：Thread A 的购物车数据丢失  ✗

  修复方式是加 ConditionExpression:
  "attribute_not_exists(cart_id)"，但现有代码没有。

  ---
  对比总结

  Race 场景: 并发添加同一产品
  MySQL: ✅ 行锁 + ON DUPLICATE KEY，数量正确合并
  DynamoDB: ❌ list_append 追加重复条目，数量不合并
  ────────────────────────────────────────
  Race 场景: 删购物车同时加商品
  MySQL: ⚠️  FK 兜底防止脏数据，但返回 500 而非 404
  DynamoDB: ✅ ConditionExpression 原子检查，返回 404
  ────────────────────────────────────────
  Race 场景: 并发创建购物车
  MySQL: ✅ AUTO_INCREMENT 保证唯一
  DynamoDB: ⚠️  UnixNano 可能碰撞，PutItem 静默覆盖

  核心差异： MySQL 把并发安全交给数据库的锁机制和约束（行锁、UNIQUE
  KEY、FK）；DynamoDB 没有锁，靠 ConditionExpression 做乐观并发控制，但现有实现的
   list_append 没有用条件写来保证 product 去重，这是设计上的一个漏洞。