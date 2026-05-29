package config

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

// sensitive_words.json 是一份本地占位敏感词词库，编译期通过 go:embed 内置到二进制。
//
// 设计说明：
//   - 仅作为同行文字弹幕的内容审核兜底，命中即整条拒绝；
//   - 词库量级控制在数百~数千条之内，朴素子串扫描即可，不引入 Aho-Corasick；
//   - 大小写不敏感：加载时 ToLower，查询前对内容 ToLower；
//   - 词库可由运维替换 JSON 文件后重编译；后续若需远端动态下发再做迭代。
//
//go:embed sensitive_words.json
var sensitiveWordsRaw []byte

var (
	sensitiveWordsOnce sync.Once
	sensitiveWordsList []string
)

func loadSensitiveWords() {
	sensitiveWordsOnce.Do(func() {
		var raw []string
		if err := json.Unmarshal(sensitiveWordsRaw, &raw); err != nil {
			sensitiveWordsList = nil
			return
		}
		seen := make(map[string]struct{}, len(raw))
		list := make([]string, 0, len(raw))
		for _, w := range raw {
			w = strings.ToLower(strings.TrimSpace(w))
			if w == "" {
				continue
			}
			if _, ok := seen[w]; ok {
				continue
			}
			seen[w] = struct{}{}
			list = append(list, w)
		}
		sensitiveWordsList = list
	})
}

// MatchSensitive 检查 content 是否命中本地敏感词词库。
//
//   - 大小写不敏感（统一 ToLower）；
//   - 命中即返回首个匹配的词与 true；未命中返回空串与 false；
//   - 空内容直接返回未命中。
//
// 注意：返回的命中词仅用于服务端日志/审计，不要直接回传给客户端，避免被反向枚举词库。
func MatchSensitive(content string) (string, bool) {
	if content == "" {
		return "", false
	}
	loadSensitiveWords()
	if len(sensitiveWordsList) == 0 {
		return "", false
	}
	lower := strings.ToLower(content)
	for _, w := range sensitiveWordsList {
		if strings.Contains(lower, w) {
			return w, true
		}
	}
	return "", false
}
