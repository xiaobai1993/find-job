# RWMutex 读写锁底层原理

---

## 一、先搞懂：读写锁解决了什么问题？

互斥锁 `Mutex` 有个问题：不管是读还是写，都要加锁，完全串行化。

但是实际场景中，**读操作远远多于写操作**，而且读操作之间是不冲突的，可以并发进行。读写锁就是为了解决这个场景而设计的。

**核心规则：**
| 当前状态 | 读请求 | 写请求 |
|---------|--------|--------|
| 无锁 | 可以加读锁 | 可以加写锁 |
| 有读锁 | 可以加读锁 | 阻塞等待所有读锁释放 |
| 有写锁 | 阻塞 | 阻塞 |

一句话总结：**读读共享，读写互斥，写写互斥。**

---

## 二、RWMutex 数据结构

```go
type RWMutex struct {
    w           Mutex  // 互斥锁，控制多个写操作之间的互斥
    writerSem   uint32 // 写等待者的信号量，最后一个读者释放锁时释放信号量
    readerSem   uint32 // 读等待者的信号量，持有写锁的协程释放锁后释放信号量
    readerCount int32  // 当前读者个数
    readerWait  int32  // 写阻塞时，排在写前面的读者个数（写需要等待这些读者读完）
}
```

**五个字段，各司其职：**
- `w`: 互斥锁，保证写写互斥，同一时间只能有一个写
- `writerSem`: 写者等待信号量，当写被读阻塞时，最后一个读者读完会释放这个信号量唤醒写
- `readerSem`: 读者等待信号量，写完成后释放这个信号量唤醒所有等待的读
- `readerCount`: 当前有多少个读者
- `readerWait`: 写锁到来时，已经持有读锁的读者数量，写需要等待这些读者都读完才能获得锁

---

## 三、四个接口实现原理

RWMutex 提供 4 个接口，这是面试必问的：

| 接口 | 作用 |
|------|------|
| Lock() | 加写锁 |
| Unlock() | 释放写锁 |
| RLock() | 加读锁 |
| RUnlock() | 释放读锁 |

下面逐个拆解实现逻辑。

---

### 1. Lock()：加写锁

加写锁需要做两件事：
1. 获取互斥锁 `w`，保证写写互斥
2. 等待所有已经存在的读者读完

**流程图：**
```
开始
  ↓
获取互斥锁 w (保证只有一个写)
  ↓
readerCount > 0 ?
  ├─ 是 → 把 readerCount 拷贝到 readerWait，阻塞等待 readerWait 个读者读完
  └─ 否 → 直接获得写锁
  ↓
结束
```

**最精妙的设计：readerCount 减 2^30 变成负数，标记有写在等待**

```go
const rwmutexMaxReaders = 1 << 30  // 2的30次方，约10亿

func (rw *RWMutex) Lock() {
    // 第一步：获取互斥锁，保证写写互斥
    rw.w.Lock()

    // 第二步：readerCount 减 rwmutexMaxReaders，变成负数
    // 这就告诉后面来的 RLock()：有写在等待，你们也得等
    r := atomic.AddInt32(&rw.readerCount, -rwmutexMaxReaders) + rwmutexMaxReaders

    // 第三步：如果有读者在读，就等待这些读者读完
    if r != 0 && atomic.AddInt32(&rw.readerWait, r) != 0 {
        runtime_Semacquire(&rw.writerSem)  // 阻塞等待，最后一个读者读完会唤醒我
    }
}
```

**这个负数设计太妙了！**
- readerCount 同时承担两个职责：记录读者数量 + 标记有没有写在等待
- 当 readerCount 是负数时，说明有写锁在等待，新来的读也得排队
- 不需要额外的字段，一个 int32 干了两件事

---

### 2. Unlock()：释放写锁

释放写锁需要做两件事：
1. 唤醒所有因为读被阻塞的协程
2. 释放互斥锁 `w`

**流程图：**
```
开始
  ↓
readerCount 加 rwmutexMaxReaders，变回正数
  ↓
有 readerCount 个读在等待写锁释放？
  ├─ 是 → 循环释放 readerCount 次 readerSem，唤醒所有等待的读
  └─ 否 → 不需要唤醒
  ↓
释放互斥锁 w
  ↓
结束
```

```go
func (rw *RWMutex) Unlock() {
    // 第一步：readerCount 加回来，变回正数，告诉大家写锁要释放了
    r := atomic.AddInt32(&rw.readerCount, rwmutexMaxReaders)

    // 第二步：唤醒所有等待写锁释放的读者
    for i := 0; i < int(r); i++ {
        runtime_Semrelease(&rw.readerSem, false, 0)
    }

    // 第三步：释放互斥锁
    rw.w.Unlock()
}
```

---

### 3. RLock()：加读锁

加读锁需要做两件事：
1. readerCount++
2. 如果有写锁在等待，阻塞等写锁释放

**流程图：**
```
开始
  ↓
readerCount++
  ↓
有写锁在等待？（readerCount 是负数）
  ├─ 是 → 阻塞在 readerSem，等待写锁释放
  └─ 否 → 直接获得读锁，返回
  ↓
结束
```

```go
func (rw *RWMutex) RLock() {
    // readerCount + 1
    if atomic.AddInt32(&rw.readerCount, 1) < 0 {
        // 结果是负数！说明有写锁在等待，我得等
        runtime_Semacquire(&rw.readerSem)
    }
}
```

**太简洁了！** 就两行代码，原子加一下，负数就等信号量，完事。

---

### 4. RUnlock()：释放读锁

释放读锁需要做两件事：
1. readerCount--
2. 如果是最后一个读者，并且有写在等待，唤醒写

**流程图：**
```
开始
  ↓
readerCount--
  ↓
readerCount-- 后是负数？（说明有写在等待）
  ├─ 是 → readerWait--
  │        readerWait == 0 ?
  │        ├─ 是 → 所有排在写前面的读者都读完了，释放 writerSem 唤醒写
  │        └─ 否 → 还有读者没读完，继续等
  └─ 否 → 直接返回
  ↓
结束
```

```go
func (rw *RWMutex) RUnlock() {
    // readerCount - 1
    if r := atomic.AddInt32(&rw.readerCount, -1); r < 0 {
        // 负数，说明有写在等待，检查是不是最后一个读者
        rw.rUnlockSlow(r)
    }
}

func (rw *RWMutex) rUnlockSlow(r int32) {
    // readerWait - 1，如果变成 0，说明是最后一个读者，可以唤醒写了
    if atomic.AddInt32(&rw.readerWait, -1) == 0 {
        // 最后一个读者读完了，唤醒等待的写
        runtime_Semrelease(&rw.writerSem, false, 0)
    }
}
```

---

## 四、四个核心场景完整分析

### 场景 1：写操作如何阻止其他写操作？

答案：靠互斥锁 `w`。

写锁必须先获取 `w`，而 `w` 是互斥锁，同一时间只能有一个 goroutine 持有，自然就实现了写写互斥。

---

### 场景 2：写操作如何阻止读操作？

答案：靠 `readerCount` 减 `rwmutexMaxReaders` 变成负数。

写锁来了之后，`readerCount` 减去 2^30 变成负数。后面新来的 RLock() 执行 `readerCount++` 发现结果是负数，就知道有写在等待，自己也得阻塞等。

**这个负数设计是整个 RWMutex 最精妙的地方！** 一个字段同时干两件事。

---

### 场景 3：读操作如何阻止写操作？

答案：写锁看到 `readerCount > 0` 就会把值拷贝到 `readerWait`，然后等 `readerWait` 变成 0。

写锁来了之后，发现还有读者在读，不会直接忙等，而是把当前读者数量记到 `readerWait` 里，然后阻塞在信号量上。

每个读者释放锁的时候都会 `readerWait--`，最后一个读者释放时发现 `readerWait` 变成 0 了，就释放信号量唤醒写锁。

---

### 场景 4：为什么写锁不会被饿死？

这是面试高频追问！

很多人担心：如果读特别多，源源不断的读进来，写锁是不是永远拿不到？

**答案：不会饿死。**

为什么？因为写锁来了之后，`readerCount` 就变成负数了，后面新来的读看到是负数，也会跟着一起等，不会让读无限增加把写饿死。

设计非常巧妙：
- 写锁来了，相当于在门口放了个「正在写，后面的读先等一等」的牌子
- 已经在里面的读可以继续读，读完再让写进去
- 但新来的读看到牌子，就一起在外面等，不会插队
- 等里面的读都读完了，写先进去
- 写出来之后，再把所有等在外面的读一起放进去

**相当于把连续的读切割成了一批一批，不会让写无限等待。**

---

## 五、RWMutex 常见坑

### 坑 1：不要重复解锁

和 Mutex 一样，重复 Unlock() / RUnlock() 会直接 panic。

### 坑 2：读锁里调用写锁，死锁

```go
// ❌ 死锁！
rw.RLock()
rw.Lock()  // 写锁等读锁释放，读锁等写锁获取，互相等
```

### 坑 3：不要把 RWMutex 当普通 Mutex 用

如果读写差不多，甚至写更多，RWMutex 的性能比普通 Mutex 还差，因为它多了很多原子操作和判断。

**只有读多写少的场景才适合用 RWMutex！**

支付系统的配置缓存、字典缓存这种读特别多，写特别少的场景，用 RWMutex 性能提升非常明显。

---

## 六、面试高频问答

| 问题 | 答案 |
|------|------|
| RWMutex 几个字段？分别什么作用？ | 5个字段。w：互斥锁保证写写互斥；writerSem：写等待信号量；readerSem：读等待信号量；readerCount：当前读者数；readerWait：写需要等待的读者数。 |
| 读写锁互斥规则是什么？ | 读读共享，读写互斥，写写互斥。 |
| readerCount 为什么要减 2^30 变成负数？ | 这是最精妙的设计：负数标记有写锁在等待，新来的读看到负数也会阻塞等待，防止写被饿死。一个字段同时记录读者数量和写等待状态。 |
| 写锁会被饿死吗？ | 不会。写锁来了之后 readerCount 变成负数，新来的读看到负数也会等，不会无限插队。等已有的读读完，写就能拿到锁。 |
| 写锁等待的时候，新来的读是立即拿到锁还是一起等？ | 一起等。写锁来了之后 readerCount 变成负数，新来的 RLock() 看到负数就会阻塞在 readerSem 等写锁释放。 |
| 最后一个读者怎么知道要唤醒写锁？ | readerWait 记录了写锁到来时已经存在的读者数量，每个读者释放锁时 readerWait--，变成 0 说明是最后一个读者，释放 writerSem 唤醒写锁。 |
| 写锁释放时怎么唤醒所有等待的读？ | 写锁释放时 readerCount 加回 2^30 变成正数，然后循环 readerCount 次释放 readerSem，唤醒所有等待的读者。 |
| 什么场景适合用 RWMutex？ | 读多写少的场景，比如缓存、配置中心。如果读写差不多甚至写更多，用普通 Mutex 性能更好。 |

---

## 七、一句话总结

> RWMutex 用一个 readerCount 字段同时干两件事：正数记录读者数，负数标记有写在等待；读多写少性能好，写不会被饿死，Go 标准库设计的典范。
