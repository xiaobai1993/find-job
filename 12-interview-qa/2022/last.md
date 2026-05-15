slice底层是怎么实现的，包含哪些属性
slice扩容的机制是什么
new和make的区别是什么
go语言的函数参数是怎么样传递
go语言闭包使用注意的问题
for i:=0;i<10;i++{
go func(){
fmt.println(i)      
}()
}
//这个代码输出是什么
go里面的读写锁和互斥锁的区别是什么，应用哪些场景
go里面需要反复创建销毁对象有没有什么方式优化，考sync.Pool
go协程并发控制的几种方式：sync.waitGroup,channel,context
go slice是线程安全的么，map是线程安全的么
go的内存逃逸是怎么回事
进程和线程和协程的区别，说下MPG的实现机制和原理
channel的遍历场景，for range遍历channel怎么退出
有哪些场景可能引发panic
go里面的继承是怎么实现的，go嵌套的struct里面的属性如果是未导出的话，能否被访问到。
go实现多个协程获取同一份数据，如果有一个获取到，其他协程就停止，（这个好未来面试官说go语言包里面提供了，记不清哪一个包了）。
signal := make(chan struct{})
var flag int32 = 0

f := func() {

for ; ; {
select {
case <-signal:
fmt.Println("收到信号退出了")
return
default:
//耗时获取数据
time.Sleep(time.Millisecond * time.Duration(rand.Int()%4))
if atomic.CompareAndSwapInt32(&flag,0,1){
close(signal)
fmt.Println("我是唯一获取成功的")
}else {
fmt.Println("虽然获取到了，但是有人已经成功了")

         }
         return
      }
}
}

for i:=0;i<10;i++{
go f()
}

time.Sleep(time.Second * 5)
go实现访问其他业务方数据，如果超过一定时间就不在等待，直接返回了。
sync.map,sync.cond,sync.pool的作用和实现原理
互斥锁、读写锁是如何实现的，加锁和解锁的过程是什么样子的。
谈谈GC和各个go语言版本中对GC的优化过程
一个url输入到浏览器，谈谈哪些环节可能发生缓存，越详细越好
Thinkphp框架里面的哪一个操作用到了观察者模式
装饰模式和职责链模式的区别和应用场景
交易系统的目前能够承受的访问量是多少
交易系统中的事务一致性怎么保证，订单、库存、预支付单，当前的实现有什么弊端，更合理的方案有哪些？
TCC是什么，它和2段提交和3段提交有什么区别，适用与哪些场景？
go系统里面的监控线程是怎么避免单个协程执行时间过长，进行任务切换的？
go的map的读取和写入是怎么实现的？
go里面什么时候用用panic，什么时候用error，go语言里面里面的panic场景有哪些考量
php里面一般是返回error还是抛异常
cap是什么，mysql主从是ca，还是ap
协程和线程那种可能会发生CPU切换，在什么场景下会产生
mysql的binlog有几种模式，优缺点是什么
mysql的rr隔离级别是怎么实现的
mysql的间隙锁是什么
mysql有哪些锁，并介绍用途
mysql在什么情况下用乐观锁什么情况用悲观锁，考量是什么？
mysql的binlog复制过程中一共涉及了几个线程，作用是什么
mysql的undo_log和redo_log的作用是什么？
mysql崩溃后重启后是怎么样的加载顺序，（undo_log和redo_log，binlog），是怎么保证不事务提交不丢失数据的。
redolog和binlog的写入顺序是怎么样的，如何保证binlog写入失败的时候还能保证数据正确性。
redolog里面放的是什么，它写入磁盘规则是什么
redolog的作用是什么？（除了保证acid的持久性以外还有什么作用）
mvcc和undolog的关系，mvcc是如何做到非锁定读的，结合read repeated和read commit，是如何找到上一个版本的内容的，说一下查询的规则，比如只查询比事务id小的。
谈谈channel在工作中的使用场景
谈谈trade的稳定性做了那些事情（除了平台本身提供的一些能力以外）
画一个符合索引的mysql的b+树的大致结构
go里面有哪些协程并发控制手段
谈谈redis的跳跃表和mysql里面的b+树这两个的异同点
谈谈你对go、java、php这三个语言用起来的感受
实现LRU要求用单链表，不能用map和双向链表:https://leetcode-cn.com/problems/lru-cache/
两个栈实现一个队列
链表求和：https://leetcode-cn.com/problems/add-two-numbers-ii/，不能用栈，最多只能多用一个新增节点空间
谈谈reids的集群方案有哪些，redis cluster和codis他们的区别和优缺点是什么
poll epoll select的区别

select * from table where id in (1,2,3) 是怎么走的索引
写一个字符串转int的函数 简单的就可以，固定把 "1234"转换成1234
给一个一致性哈希算法的案例场景，让设计虚拟节点和非虚拟节点的个数
写一个最简单的二分查找法
redis的分布式锁有什么缺陷，如何防止执行任务时间过长导致锁被其他任务抢占的问题
服务发现的实现原理和作用（简单聊一聊）

--- shopee

mysql的引擎有哪些
操作系统调度的基本单位是什么
mysql索引用了哪些数据结构场景是什么
设计一个分布式限流器（参考下欢总写的go-leakbucket）
mysql的优化手段可以想到哪些，不用展开说，说大概方向就可以。
redis的zset是怎么实现的
http和tcp的区别
不同版本的http有什么特性，（http3.0没有答出来）
nginx的原理是怎么实现的
简述以下epoll是什么，以怎么样的机制对外使用的
操作系统调度的基本单位是什么
链表和数组有什么区别，
算法题：找到一个字符串里面的长度大于等于4的回文子串，跑通测试案例就可以。

-- 头条

设计一个分布式限流器
分布式事务
系统稳定性：服务降级、熔断、超时控制
ACID是怎么实现的

func cal(value int, arr []int) int {
ret := 0
sort.SliceStable(arr, func(i, j int) bool {
return arr[i] < arr[j]
})
valueArr := make([]int, 0)
strV := strconv.Itoa(value)
for i := 0; i < len(strV); i++ {
valueArr = append(valueArr, int(strV[i]-'0'))
}
for i := 0; i < len(valueArr); i++ {
selectV := -1
for j := 0; j < len(arr); j++ {
if arr[j]<= valueArr[i] {
selectV = arr[j]
}
}
if selectV == -1   {
t := ret
if i >= 1 && t % 10 <= valueArr[i-1] {
t = t / 10 * 10 + arr[len(arr)-1]
}
for t <= value {
t = t * 10 + arr[len(arr)-1]
}
return t / 10
}else{
ret = ret * 10 + selectV
}
}
return ret
}

func
--- OPAY

同步、异步、阻塞、 非阻塞
10万QPS实时统计top5的数据
超时控制的手段有哪些
谈谈项目

--- 蓝湖
讲讲项目
json的encode和decode优化方案
查找无序数组里面第K小的数（堆排序）
--- 得到二面
如果不让用延迟消息队列实现订单15分钟未支付关单操作

