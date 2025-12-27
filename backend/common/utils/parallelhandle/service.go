package parallelhandle

import (
	"log"
	"sync"
	"time"
)

// CommonParallelExecutorService 通用并行执行工具类，适用于各种需要并行处理的场景
type CommonParallelExecutorService struct{}

// ErrorHandler 错误处理函数类型，接收数据项和错误，返回处理结果
type ErrorHandler func(item interface{}, err error) interface{}

// ExecuteInParallel 并行执行指定的函数处理一组数据项
// items: 需要处理的数据项集合
// workerFunction: 处理单个数据项的函数，接收一个数据项，返回处理结果
// maxWorkers: 最大并行工作线程数
// timeout: 任务超时时间(秒)，为0表示无限等待
// errorHandler: 错误处理函数，接收数据项和异常，返回任意值作为该项的结果
// 返回值: 包含数据项和对应处理结果的映射
func (p *CommonParallelExecutorService) ExecuteInParallel(
	items []interface{},
	workerFunction func(interface{}) interface{},
	maxWorkers int,
	timeout time.Duration,
	errorHandler ErrorHandler,
) map[interface{}]interface{} {
	results := make(map[interface{}]interface{})
	var resultsMutex sync.Mutex

	// 使用通道控制最大并行度
	semaphore := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	// 如果设置了超时，创建超时通道
	var timeoutChan <-chan time.Time
	if timeout > 0 {
		timeoutChan = time.After(timeout * time.Second)
	}

	// 创建完成信号通道
	done := make(chan struct{})

	// 并行处理所有项
	for _, item := range items {
		wg.Add(1)
		go func(item interface{}) {
			defer wg.Done()

			// 获取信号量
			select {
			case semaphore <- struct{}{}:
				// 成功获取信号量，继续执行
				defer func() { <-semaphore }() // 完成后释放信号量
			case <-timeoutChan:
				// 超时
				log.Printf("获取信号量超时，跳过处理项 %v", item)
				return
			}

			// 执行工作函数并处理可能的错误
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("处理项 %v 时发生异常: %v", item, r)
						if errorHandler != nil {
							resultsMutex.Lock()
							results[item] = errorHandler(item, nil)
							resultsMutex.Unlock()
						}
					}
				}()

				// 执行工作函数
				result := workerFunction(item)

				// 保存结果
				resultsMutex.Lock()
				results[item] = result
				resultsMutex.Unlock()
			}()
		}(item)
	}

	// 等待所有工作完成或超时
	go func() {
		wg.Wait()
		close(done)
	}()

	// 等待完成或超时
	select {
	case <-done:
		// 所有工作完成
		return results
	case <-timeoutChan:
		// 超时
		log.Printf("部分任务执行超时(超过%v秒)", timeout)
		return results
	}
}

// MapInParallel 并行映射函数到一组数据项（类似于内置map函数的并行版本）
// items: 需要处理的数据项集合
// mapperFunction: 映射函数，接收一个数据项，返回映射结果
// maxWorkers: 最大并行工作线程数
// timeout: 任务超时时间(秒)，为0表示无限等待
// 返回值: 包含映射结果的切片，顺序与输入项一致
func (p *CommonParallelExecutorService) MapInParallel(
	items []interface{},
	mapperFunction func(interface{}) interface{},
	maxWorkers int,
	timeout time.Duration,
) []interface{} {
	results := make([]interface{}, len(items))
	var resultsMutex sync.Mutex

	// 使用通道控制最大并行度
	semaphore := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	// 如果设置了超时，创建超时通道
	var timeoutChan <-chan time.Time
	if timeout > 0 {
		timeoutChan = time.After(timeout * time.Second)
	}

	// 创建完成信号通道
	done := make(chan struct{})

	// 并行处理所有项
	for i, item := range items {
		wg.Add(1)
		go func(i int, item interface{}) {
			defer wg.Done()

			// 获取信号量
			select {
			case semaphore <- struct{}{}:
				// 成功获取信号量，继续执行
				defer func() { <-semaphore }() // 完成后释放信号量
			case <-timeoutChan:
				// 超时
				return
			}

			// 执行映射函数
			result := mapperFunction(item)

			// 保存结果到对应位置
			resultsMutex.Lock()
			results[i] = result
			resultsMutex.Unlock()
		}(i, item)
	}

	// 等待所有工作完成或超时
	go func() {
		wg.Wait()
		close(done)
	}()

	// 等待完成或超时
	select {
	case <-done:
		// 所有工作完成
		return results
	case <-timeoutChan:
		// 超时
		log.Printf("部分任务执行超时(超过%v秒)", timeout)
		return results
	}
}

// FilterInParallel 并行过滤一组数据项（类似于内置filter函数的并行版本）
// items: 需要处理的数据项集合
// filterFunction: 过滤函数，返回true表示保留该项
// maxWorkers: 最大并行工作线程数
// timeout: 任务超时时间(秒)，为0表示无限等待
// 返回值: 过滤后的数据项切片
func (p *CommonParallelExecutorService) FilterInParallel(
	items []interface{},
	filterFunction func(interface{}) bool,
	maxWorkers int,
	timeout time.Duration,
) []interface{} {
	type indexedResult struct {
		index  int
		passed bool
	}

	results := make([]indexedResult, 0, len(items))
	var resultsMutex sync.Mutex

	// 使用通道控制最大并行度
	semaphore := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	// 如果设置了超时，创建超时通道
	var timeoutChan <-chan time.Time
	if timeout > 0 {
		timeoutChan = time.After(timeout * time.Second)
	}

	// 创建完成信号通道
	done := make(chan struct{})

	// 并行处理所有项
	for i, item := range items {
		wg.Add(1)
		go func(i int, item interface{}) {
			defer wg.Done()

			// 获取信号量
			select {
			case semaphore <- struct{}{}:
				// 成功获取信号量，继续执行
				defer func() { <-semaphore }() // 完成后释放信号量
			case <-timeoutChan:
				// 超时
				return
			}

			// 执行过滤函数
			passed := filterFunction(item)

			// 如果通过过滤，保存结果
			if passed {
				resultsMutex.Lock()
				results = append(results, indexedResult{index: i, passed: true})
				resultsMutex.Unlock()
			}
		}(i, item)
	}

	// 等待所有工作完成或超时
	go func() {
		wg.Wait()
		close(done)
	}()

	// 等待完成或超时
	select {
	case <-done:
		// 所有工作完成
		// 按原始顺序返回通过过滤的项
		filteredItems := make([]interface{}, 0, len(results))
		for _, res := range results {
			if res.passed {
				filteredItems = append(filteredItems, items[res.index])
			}
		}
		return filteredItems
	case <-timeoutChan:
		// 超时
		log.Printf("部分任务执行超时(超过%v秒)", timeout)
		// 返回已完成的过滤结果
		filteredItems := make([]interface{}, 0, len(results))
		for _, res := range results {
			if res.passed {
				filteredItems = append(filteredItems, items[res.index])
			}
		}
		return filteredItems
	}
}

// ForEachInParallel 并行对一组数据项执行操作，不关心返回值（类似于forEach操作）
// items: 需要处理的数据项集合
// actionFunction: 对每项执行的操作函数
// maxWorkers: 最大并行工作线程数
// timeout: 任务超时时间(秒)，为0表示无限等待
func (p *CommonParallelExecutorService) ForEachInParallel(
	items []interface{},
	actionFunction func(interface{}),
	maxWorkers int,
	timeout time.Duration,
) {
	// 使用通道控制最大并行度
	semaphore := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	// 如果设置了超时，创建超时通道
	var timeoutChan <-chan time.Time
	if timeout > 0 {
		timeoutChan = time.After(timeout * time.Second)
	}

	// 创建完成信号通道
	done := make(chan struct{})

	// 并行处理所有项
	for _, item := range items {
		wg.Add(1)
		go func(item interface{}) {
			defer wg.Done()

			// 获取信号量
			select {
			case semaphore <- struct{}{}:
				// 成功获取信号量，继续执行
				defer func() { <-semaphore }() // 完成后释放信号量
			case <-timeoutChan:
				// 超时
				return
			}

			// 执行操作函数
			actionFunction(item)
		}(item)
	}

	// 等待所有工作完成或超时
	go func() {
		wg.Wait()
		close(done)
	}()

	// 等待完成或超时
	select {
	case <-done:
		// 所有工作完成
		return
	case <-timeoutChan:
		// 超时
		log.Printf("部分任务执行超时(超过%v秒)", timeout)
		return
	}
}

// ProcessInParallel 使用进程池并行处理CPU密集型任务
// 在Go中，我们使用goroutines而不是进程，但保留该方法以保持API兼容
// items: 需要处理的数据项集合
// processFunction: 处理函数
// maxWorkers: 最大并行工作线程数
// useProcesses: 在Go中忽略此参数，始终使用goroutines
// timeout: 任务超时时间(秒)，为0表示无限等待
// 返回值: 包含数据项和对应处理结果的映射
func (p *CommonParallelExecutorService) ProcessInParallel(
	items []interface{},
	processFunction func(interface{}) interface{},
	maxWorkers int,
	useProcesses bool, // 在Go中忽略此参数
	timeout time.Duration,
) map[interface{}]interface{} {
	// 在Go中，我们直接使用goroutines，忽略useProcesses参数
	// 这个方法实际上与ExecuteInParallel几乎相同，但保留它以兼容Python版本的API
	results := make(map[interface{}]interface{})
	var resultsMutex sync.Mutex

	// 使用通道控制最大并行度
	semaphore := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	// 如果设置了超时，创建超时通道
	var timeoutChan <-chan time.Time
	if timeout > 0 {
		timeoutChan = time.After(timeout * time.Second)
	}

	// 创建完成信号通道
	done := make(chan struct{})

	// 并行处理所有项
	for _, item := range items {
		wg.Add(1)
		go func(item interface{}) {
			defer wg.Done()

			// 获取信号量
			select {
			case semaphore <- struct{}{}:
				// 成功获取信号量，继续执行
				defer func() { <-semaphore }() // 完成后释放信号量
			case <-timeoutChan:
				// 超时
				return
			}

			// 执行处理函数并处理可能的错误
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("处理项 %v 时发生异常: %v", item, r)
						resultsMutex.Lock()
						results[item] = r
						resultsMutex.Unlock()
					}
				}()

				// 执行处理函数
				result := processFunction(item)

				// 保存结果
				resultsMutex.Lock()
				results[item] = result
				resultsMutex.Unlock()
			}()
		}(item)
	}

	// 等待所有工作完成或超时
	go func() {
		wg.Wait()
		close(done)
	}()

	// 等待完成或超时
	select {
	case <-done:
		// 所有工作完成
		return results
	case <-timeoutChan:
		// 超时
		log.Printf("部分任务执行超时(超过%v秒)", timeout)
		return results
	}
}

// 创建一个单例实例供外部使用
var Service = &CommonParallelExecutorService{} 