package components

import (
	"fmt"
	"sort"
)

// resolve 对组件进行拓扑排序，返回启动顺序
// 同一拓扑层内按名称字典序排序，确保启动顺序确定可预测
func resolve(comps map[string]Component) ([]string, error) {
	// 构建邻接表和入度
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // dep → 依赖它的组件列表

	for name := range comps {
		if _, ok := inDegree[name]; !ok {
			inDegree[name] = 0
		}
	}

	for name, c := range comps {
		for _, dep := range c.Depends() {
			if _, ok := comps[dep]; !ok {
				return nil, fmt.Errorf("components: %q depends on %q which is not registered", name, dep)
			}
			inDegree[name]++
			dependents[dep] = append(dependents[dep], name)
		}
	}

	// Kahn 算法 BFS 拓扑排序；同一入度层内按字典序处理，保证结果稳定
	var order []string
	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		// 收集新解锁的组件，按字典序加入队列，保证下一轮处理顺序稳定
		var newlyUnlocked []string
		for _, dep := range dependents[cur] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				newlyUnlocked = append(newlyUnlocked, dep)
			}
		}
		sort.Strings(newlyUnlocked)
		queue = append(queue, newlyUnlocked...)
	}

	if len(order) != len(comps) {
		return nil, fmt.Errorf("components: circular dependency detected")
	}

	return order, nil
}
