# 208. 实现 Trie (前缀树)

> LeetCode 链接：https://leetcode.cn/problems/implement-trie-prefix-tree/

## 题目
实现 Trie（前缀树）：支持 `insert(word)`、`search(word)`、`startsWith(prefix)` 操作。

## 最易理解的方法：数组型 Trie 节点

**核心思路：**
每个节点包含：
- `children[26]`：指向 26 个字母的子节点
- `isEnd`：是否是某个单词的结尾

```
插入 "apple":
root → a → p → p → l → e(isEnd=true)

search "apple" → 遍历到e, isEnd=true → true
search "app" → 遍历到p(第二个), isEnd=false → false
startsWith "app" → 遍历到p(第二个), 存在 → true
```

## Go 实现

```go
type Trie struct {
    children [26]*Trie
    isEnd    bool
}

func Constructor() Trie {
    return Trie{}
}

func (t *Trie) Insert(word string) {
    node := t
    for _, ch := range word {
        idx := ch - 'a'
        if node.children[idx] == nil {
            node.children[idx] = &Trie{}
        }
        node = node.children[idx]
    }
    node.isEnd = true
}

func (t *Trie) Search(word string) bool {
    node := t.searchPrefix(word)
    return node != nil && node.isEnd
}

func (t *Trie) StartsWith(prefix string) bool {
    return t.searchPrefix(prefix) != nil
}

func (t *Trie) searchPrefix(prefix string) *Trie {
    node := t
    for _, ch := range prefix {
        idx := ch - 'a'
        if node.children[idx] == nil {
            return nil
        }
        node = node.children[idx]
    }
    return node
}
```

## 复杂度
- Insert/Search/StartsWith：O(m)，m 为字符串长度
- 空间：O(n × 26)

## 记忆口诀
> 节点存 children[26] 和 isEnd；search=找到前缀节点且isEnd；startsWith=只需找到前缀节点即可。
