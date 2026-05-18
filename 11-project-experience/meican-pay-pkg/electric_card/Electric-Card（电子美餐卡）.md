电子餐卡相关接口，可用于支持嘀卡的企业消费。

1.  电子餐卡消费码 code 生成算法-V1：

1. 数据准备
    0. id: 通过本 `electricCard/apiKey` 获得
    1. key: 通过本 `electricCard/apiKey` 获得
    3. OTP: 使用 https://github.com/robbiev/two-factor-auth 算法(Android 中使用：https://github.com/j256/java-two-factor-auth )，以 key 为秘钥，源码中的 `input`
    4. prime_small: 68718952447
    5. prime_big: 4398050705407
    6. n: timestamp(单位: 秒) % 30

2. 公式: code = prime_small * n + id + prime_big * OTP

3. `n` 用作本地刷新

----

1.  电子餐卡消费码 code 生成算法-V2：

```
|id + otp * p|check_code|
-------------|-----------
      15位          3位
```

api_key为Base32编码

1. check_code 用于完整性和时间校验，3位

2. p 为9位素数：931353139

3. number 为电子美餐卡号, 不得超过 p 的值

4. otp 由 api_key 和客户端时间加密生成，6位

5. number + otp * p 结果不足15位需要在前面补零

6. otp 算法使用的 timestamp 需要自定义元年，即 timestamp = timestamp - 1262275200000，即从北京时间 2010-1-1 开始计算

1.  算法

1. 基本算法 V1 相同

1. 实例：
      1. 设 `GET apiKey` 得到的数据为：number=63, apiKey="MZSWEZLBHA4DQLLEGY3TGLJUGBRDMLJYMNRDOLLCHAZGCM3GGM3DANLFGI======"
      1. 设客户端时间戳为：1487239131123，得到我们需要的 timestamp = 1487239131123 - 1262275200000 = 224963931123
      1. 使用 V1 的算法的 otp = 698247
      1. 计算前 15 位 = number + otp * p = 63 + 698247 * 931353139 = 650314535247396
      1. 计算后 3 位：
          1. 拼接 number 和 otp 得 s = "63698247" （反解 OTP 不足 6位前补 0 至 6位）
          1. s = base32(s) （仅限Android）
          1. 使用 V1 的算法，用 s 代替 key，timestamp 不变，得 466448，取后 3 位：448
      1. 最终支付码：650314535247396448


1.  客户端时间修改容错
      
- 客户端时间修改在前后30分钟内，可验证通过，最多需连续计算240次otp

1.  POS机离线校验

1. 解出 number 和 otp，用 POS机时间 计算 check_code
2. POS机可自己决定容忍客户端修改时间的范围

1.  POS机离线测试

|项目|测试数量|正确率|
|---|---|---|
|正确码随机修改1位|200,000|0.1%|
|正确码随机修改2位|200,000|0.1%|
|随机18位|200,000|0.1%|
|正确码在不同时间点|200,000|0.1%|

1.  POS机在线测试

|项目|测试数量|正确率|
|---|---|---|
|正确码随机修改1位|200,000|0|
|正确码随机修改2位|200,000|0|
|随机18位|200,000|0|

用于验证的数据

```
- "found number: 930878892, key: GA3TGY3FGVSGMLJUHEZWELJUGBSWCLJYGFRTQLJWGVQWMZTDGQ2TCN3FGQ======"
- "current timestamp: 1488791421696"
- "seconds remaining: 9"
- "decoded key: 073ce5df-493b-40ea-81c8-65affc4517e4"
- "OTP: 091621"
- "check code input secret: 930878892091621"
- "check code: 304"
- "left part: 085332436827211"
- "result code: 085332436827211304"
```