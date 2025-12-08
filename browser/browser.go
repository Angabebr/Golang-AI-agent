package browser

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

type Browser struct {
	ctx             context.Context
	cancel          context.CancelFunc
	allocCtx        context.Context
	allocCancel     context.CancelFunc
	keepAlive       context.Context
	keepAliveCancel context.CancelFunc
}

func NewBrowser(userDataDir string, headless bool) (*Browser, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", headless),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("disable-dev-shm-usage", false),
		chromedp.Flag("no-sandbox", false),
		chromedp.UserDataDir(userDataDir),
		chromedp.WindowSize(1920, 1080),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("profile-directory", "Default"),
		chromedp.Flag("disable-extensions", false),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("single-process", false),
		chromedp.Flag("disable-features", "VizDisplayCompositor,TranslateUI"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(format string, v ...interface{}) {
		msg := fmt.Sprintf(format, v...)

		ignorePatterns := []string{
			"could not unmarshal event",
			"unexpected end of JSON input",
			"unknown IPAddressSpace value",
			"unknown PrivateNetworkRequestPolicy value",
			"parse error",
			"cookiePart",
		}

		shouldIgnore := false
		for _, pattern := range ignorePatterns {
			if contains(msg, pattern) {
				shouldIgnore = true
				break
			}
		}

		if !shouldIgnore {
		}
	}))

	keepAliveCtx, keepAliveCancel := context.WithCancel(context.Background())

	b := &Browser{
		ctx:             ctx,
		cancel:          cancel,
		allocCtx:        allocCtx,
		allocCancel:     allocCancel,
		keepAlive:       keepAliveCtx,
		keepAliveCancel: keepAliveCancel,
	}

	if err := chromedp.Run(ctx,
		chromedp.Navigate("about:blank"),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	); err != nil {
		keepAliveCancel()
		return nil, fmt.Errorf("failed to start browser: %w\n\nВозможные причины:\n- Chrome/Chromium не установлен\n- Chrome заблокирован антивирусом\n- Недостаточно прав для запуска\n- Директория браузера занята другим процессом\n\nУстановите Chrome или Chromium: https://www.google.com/chrome/", err)
	}

	select {
	case <-ctx.Done():
		keepAliveCancel()
		return nil, fmt.Errorf("browser context was canceled after initialization")
	default:
	}

	go b.keepAliveLoop()

	return b, nil
}

func (b *Browser) Navigate(url string) error {
	select {
	case <-b.ctx.Done():
		return fmt.Errorf("browser context was canceled before navigation - keep-alive may not be working")
	default:
	}

	err := chromedp.Run(b.ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
	)

	if err != nil {
		errStr := err.Error()
		if errStr == "invalid context" || err == context.Canceled {
			return fmt.Errorf("browser context was canceled during navigation - keep-alive may not be working: %w", err)
		}
		return fmt.Errorf("failed to navigate to %s: %w", url, err)
	}

	time.Sleep(500 * time.Millisecond)

	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func (b *Browser) GetPageContent() (*PageContent, error) {
	// Проверяем, не отменен ли контекст браузера
	select {
	case <-b.ctx.Done():
		return nil, fmt.Errorf("browser context was canceled - браузер недоступен")
	default:
	}

	// Увеличиваем таймаут и добавляем повторные попытки
	maxRetries := 3
	var content PageContent
	var err error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(b.ctx, 45*time.Second)
		
		// Сначала прокручиваем страницу и ждем загрузки динамического контента
		_ = chromedp.Run(ctx,
			chromedp.Sleep(2*time.Second), // Ждем загрузки динамического контента
			chromedp.Evaluate(`
				// Прокручиваем страницу вниз для загрузки всех элементов
				window.scrollTo(0, 0);
				setTimeout(() => window.scrollTo(0, document.body.scrollHeight / 2), 100);
				setTimeout(() => window.scrollTo(0, document.body.scrollHeight), 200);
				setTimeout(() => window.scrollTo(0, 0), 300);
			`, nil),
			chromedp.Sleep(1*time.Second), // Ждем после прокрутки
		)
		
		err = chromedp.Run(ctx,
			chromedp.Evaluate(`
		(function() {
			function isVisible(el) {
				if (!el) return false;
				const style = window.getComputedStyle(el);
				return style.display !== 'none' && 
					   style.visibility !== 'hidden' && 
					   style.opacity !== '0' &&
					   el.offsetWidth > 0 && 
					   el.offsetHeight > 0;
			}
			
			function isInViewport(el) {
				if (!el) return false;
				const rect = el.getBoundingClientRect();
				return rect.top >= 0 && rect.left >= 0 && 
					   rect.bottom <= (window.innerHeight || document.documentElement.clientHeight) &&
					   rect.right <= (window.innerWidth || document.documentElement.clientWidth);
			}
			
			function getTextContent(el, maxLength) {
				if (!el) return '';
				const text = (el.innerText || el.textContent || '').trim();
				return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
			}
			
			// Умное извлечение текста - только видимая часть и важные элементы
			const bodyText = document.body.innerText || '';
			const textPreview = bodyText.length > 5000 ? bodyText.substring(0, 5000) + '...' : bodyText;
			
			// Извлечение структурированных данных - УВЕЛИЧИВАЕМ лимиты
			const links = Array.from(document.querySelectorAll('a')).slice(0, 200).map(a => {
				const text = (a.innerText || a.textContent || '').trim();
				const href = a.href;
				const visible = isVisible(a);
				return { text, href, visible };
			}).filter(l => l.visible && l.text && l.href);
			
			// Функция для получения текста кнопки, включая иконки и символы
			function getButtonText(b) {
				// Сначала пробуем обычный текст
				let text = (b.innerText || b.textContent || b.value || '').trim();
				
				// Если текста нет, пробуем aria-label, title
				if (!text) {
					text = (b.getAttribute('aria-label') || b.getAttribute('title') || '').trim();
				}
				
				// Если текста все еще нет, ищем иконки и символы
				if (!text) {
					// Ищем SVG иконки
					const svg = b.querySelector('svg');
					if (svg) {
						const svgText = svg.textContent || svg.getAttribute('aria-label') || '';
						if (svgText) text = svgText.trim();
					}
					
					// Ищем символы (+, -, ×, и т.д.)
					const symbols = b.textContent.match(/[+×−−−]/);
					if (symbols && symbols.length > 0) {
						text = symbols[0];
					}
					
					// Ищем по классам/ID для кнопок добавления
					const className = (typeof b.className === 'string' ? b.className : (b.className ? b.className.toString() : '')).toLowerCase();
					const id = (b.id || '').toLowerCase();
					if (className.includes('add') || className.includes('cart') || className.includes('basket') || 
						id.includes('add') || id.includes('cart') || id.includes('basket')) {
						text = text || '+';
					}
				}
				
				return text;
			}
			
			const buttons = Array.from(document.querySelectorAll('button, [role="button"], input[type="submit"], input[type="button"], a.button, .btn, [class*="button"], [class*="add"], [class*="cart"]')).slice(0, 200).map(b => {
				const text = getButtonText(b);
				const visible = isVisible(b);
				const enabled = !b.disabled && !b.hasAttribute('disabled');
				const tag = b.tagName.toLowerCase();
				const role = b.getAttribute('role') || '';
				// Включаем кнопки даже без текста, если они имеют специальные классы/ID
				const classNameStr = typeof b.className === 'string' ? b.className : (b.className ? b.className.toString() : '');
				const hasSpecialClass = classNameStr.toLowerCase().includes('add') || 
				                       classNameStr.toLowerCase().includes('cart') ||
				                       (b.id || '').toLowerCase().includes('add') ||
				                       (b.id || '').toLowerCase().includes('cart');
				return { text: text || (hasSpecialClass ? '+' : ''), type: tag, visible, enabled, role };
			}).filter(b => b.visible && b.enabled && (b.text || b.text === '+')); // Разрешаем кнопки с "+"
			
			const inputs = Array.from(document.querySelectorAll('input, textarea, select')).slice(0, 25).map(i => {
				const type = i.type || (i.tagName.toLowerCase() === 'textarea' ? 'textarea' : 'text');
				const placeholder = i.placeholder || '';
				const name = i.name || '';
				const id = i.id || '';
				const label = i.labels && i.labels.length > 0 ? i.labels[0].textContent : '';
				const visible = isVisible(i);
				return { type, placeholder, name, id, label, visible };
			}).filter(i => i.visible);
			
			const headings = Array.from(document.querySelectorAll('h1, h2, h3, h4')).slice(0, 25).map(h => {
				const text = (h.innerText || h.textContent || '').trim();
				return { level: h.tagName, text };
			}).filter(h => h.text);
			
			// Извлечение списков и таблиц для структурированных данных
			const lists = Array.from(document.querySelectorAll('ul, ol')).slice(0, 20).map(list => {
				const items = Array.from(list.querySelectorAll('li')).slice(0, 50).map(li => {
					return (li.innerText || li.textContent || '').trim();
				}).filter(item => item);
				return items;
			}).filter(list => list.length > 0);
			
			// Извлечение таблиц
			const tables = Array.from(document.querySelectorAll('table')).slice(0, 10).map(table => {
				const rows = Array.from(table.querySelectorAll('tr')).slice(0, 50).map(tr => {
					const cells = Array.from(tr.querySelectorAll('td, th')).map(cell => {
						return (cell.innerText || cell.textContent || '').trim();
					}).filter(cell => cell);
					return cells;
				}).filter(row => row.length > 0);
				return rows;
			}).filter(table => table.length > 0);
			
			// Извлечение элементов списка писем (специально для почтовых сервисов)
			const emailItems = [];
			// Ищем контейнеры со списками писем
			const emailContainers = document.querySelectorAll('[class*="mail"], [class*="message"], [class*="letter"], [class*="email"], [id*="mail"], [id*="message"]');
			emailContainers.forEach(container => {
				const items = Array.from(container.querySelectorAll('a, div, li, tr')).slice(0, 30);
				items.forEach(item => {
					const text = (item.innerText || item.textContent || '').trim();
					const href = item.href || '';
					if (text && text.length > 5 && text.length < 200) {
						emailItems.push({
							text: text,
							href: href,
							tag: item.tagName.toLowerCase()
						});
					}
				});
			});
			
			// Если нашли элементы писем, добавляем их в links
			if (emailItems.length > 0) {
				emailItems.forEach(item => {
					if (item.href) {
						links.push({ text: item.text, href: item.href, visible: true });
					} else {
						// Если нет href, добавляем как кнопку
						buttons.push({ text: item.text, type: item.tag, visible: true, enabled: true, role: '' });
					}
				});
			}
			
			return {
				url: window.location.href,
				title: document.title,
				text: textPreview,
				links: links.slice(0, 200), // Ограничиваем итоговый размер
				buttons: buttons.slice(0, 150),
				inputs: inputs,
				headings: headings,
				lists: lists,
				tables: tables
			};
		})()
		`, &content),
		)
		
		cancel()
		
		if err == nil {
			return &content, nil
		}
		
		// Проверяем, не отменен ли контекст браузера
		select {
		case <-b.ctx.Done():
			return nil, fmt.Errorf("browser context was canceled - браузер недоступен")
		default:
		}
		
		// Если это не последняя попытка, ждем перед повтором
		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to extract page content after %d attempts: %w", maxRetries, err)
	}

	return &content, nil
}

// GetPageSummary возвращает краткое описание страницы для экономии токенов
func (b *Browser) GetPageSummary() (string, error) {
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()

	var summary struct {
		URL      string   `json:"url"`
		Title    string   `json:"title"`
		MainText string   `json:"main_text"`
		KeyElements []string `json:"key_elements"`
	}

	err := chromedp.Run(ctx,
		chromedp.Evaluate(`
		(function() {
			const url = window.location.href;
			const title = document.title;
			
			// Извлекаем только ключевой текст (первые 2000 символов)
			const bodyText = (document.body.innerText || '').substring(0, 2000);
			
			// Ключевые элементы страницы
			const keyElements = [];
			
			// Заголовки
			const h1 = document.querySelector('h1');
			if (h1) keyElements.push('H1: ' + h1.innerText.trim());
			
			// Основные кнопки и ссылки
			const mainButtons = Array.from(document.querySelectorAll('button, [role="button"]')).slice(0, 5);
			mainButtons.forEach(btn => {
				const text = (btn.innerText || btn.textContent || '').trim();
				if (text) keyElements.push('Button: ' + text);
			});
			
			// Основные ссылки
			const mainLinks = Array.from(document.querySelectorAll('a')).slice(0, 5);
			mainLinks.forEach(link => {
				const text = (link.innerText || link.textContent || '').trim();
				if (text && link.offsetParent !== null) {
					keyElements.push('Link: ' + text);
				}
			});
			
			return {
				url: url,
				title: title,
				main_text: bodyText,
				key_elements: keyElements
			};
		})()
		`, &summary),
	)

	if err != nil {
		return "", fmt.Errorf("failed to get page summary: %w", err)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("URL: %s\n", summary.URL))
	result.WriteString(fmt.Sprintf("Title: %s\n", summary.Title))
	if summary.MainText != "" {
		result.WriteString(fmt.Sprintf("Text: %s\n", summary.MainText))
	}
	if len(summary.KeyElements) > 0 {
		result.WriteString("Key elements:\n")
		for _, el := range summary.KeyElements {
			result.WriteString(fmt.Sprintf("  - %s\n", el))
		}
	}

	return result.String(), nil
}

// GetQuickPageInfo возвращает только базовую информацию о странице (быстро, без сложной обработки)
func (b *Browser) GetQuickPageInfo() (*QuickPageInfo, error) {
	// Проверяем, не отменен ли контекст браузера
	select {
	case <-b.ctx.Done():
		return nil, fmt.Errorf("browser context was canceled - браузер недоступен")
	default:
	}

	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()

	// Прокручиваем страницу для загрузки элементов
	_ = chromedp.Run(ctx,
		chromedp.Sleep(1*time.Second),
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight / 2);`, nil),
		chromedp.Sleep(500*time.Millisecond),
	)

	var info QuickPageInfo

	err := chromedp.Run(ctx,
		chromedp.Evaluate(`
		(function() {
			function isVisible(el) {
				if (!el) return false;
				const style = window.getComputedStyle(el);
				return style.display !== 'none' && 
					   style.visibility !== 'hidden' && 
					   style.opacity !== '0' &&
					   el.offsetWidth > 0 && 
					   el.offsetHeight > 0;
			}
			
			// Увеличиваем количество ссылок для быстрого метода
			const links = Array.from(document.querySelectorAll('a')).slice(0, 100).map(a => {
				const text = (a.innerText || a.textContent || '').trim();
				const href = a.href;
				if (isVisible(a) && text && href) {
					return { text, href };
				}
				return null;
			}).filter(l => l !== null);
			
			// Функция для получения текста кнопки, включая иконки
			function getButtonText(b) {
				let text = (b.innerText || b.textContent || b.value || '').trim();
				if (!text) {
					text = (b.getAttribute('aria-label') || b.getAttribute('title') || '').trim();
				}
				if (!text) {
					// Ищем символы (+, -, ×)
					const symbols = b.textContent.match(/[+×−−−]/);
					if (symbols && symbols.length > 0) {
						text = symbols[0];
					}
					// Ищем по классам для кнопок добавления
					const className = (typeof b.className === 'string' ? b.className : (b.className ? b.className.toString() : '')).toLowerCase();
					const id = (b.id || '').toLowerCase();
					if (className.includes('add') || className.includes('cart') || id.includes('add') || id.includes('cart')) {
						text = '+';
					}
				}
				return text;
			}
			
			// Увеличиваем количество кнопок и ищем кнопки с иконками
			const buttons = Array.from(document.querySelectorAll('button, [role="button"], input[type="submit"], input[type="button"], [class*="add"], [class*="cart"]')).slice(0, 150).map(b => {
				const text = getButtonText(b);
				if (isVisible(b) && !b.disabled && text) {
					return text;
				}
				return null;
			}).filter(b => b !== null);
			
			return {
				url: window.location.href,
				title: document.title,
				links: links,
				buttons: buttons
			};
		})()
		`, &info),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get quick page info: %w", err)
	}

	return &info, nil
}

type QuickPageInfo struct {
	URL     string   `json:"url"`
	Title   string   `json:"title"`
	Links   []Link   `json:"links"`
	Buttons []string `json:"buttons"`
}

func (b *Browser) ClickElement(selector string) error {
	// Проверяем, не отменен ли контекст браузера
	select {
	case <-b.ctx.Done():
		return fmt.Errorf("browser context was canceled - браузер недоступен")
	default:
	}

	ctx, cancel := context.WithTimeout(b.ctx, 20*time.Second)
	defer cancel()

	return chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	)
}

func (b *Browser) ClickByText(text string) error {
	// Проверяем, не отменен ли контекст браузера
	select {
	case <-b.ctx.Done():
		return fmt.Errorf("browser context was canceled - браузер недоступен")
	default:
	}

	ctx, cancel := context.WithTimeout(b.ctx, 20*time.Second)
	defer cancel()

	escapedText := escapeJSString(text)

	script := fmt.Sprintf(`
		(function() {
			const searchText = '%s';
			const searchLower = searchText.toLowerCase().trim();
			
			function isVisible(el) {
				if (!el) return false;
				const style = window.getComputedStyle(el);
				return style.display !== 'none' && 
					   style.visibility !== 'hidden' && 
					   style.opacity !== '0' &&
					   el.offsetWidth > 0 && 
					   el.offsetHeight > 0;
			}
			
			function isClickable(el) {
				if (!el) return false;
				const tag = el.tagName;
				const role = el.getAttribute('role');
				const clickable = el.onclick || el.getAttribute('onclick');
				const hasPointer = window.getComputedStyle(el).cursor === 'pointer';
				const className = (typeof el.className === 'string' ? el.className : (el.className ? el.className.toString() : '')).toLowerCase();
				const id = (el.id || '').toLowerCase();
				
				// Стандартные кнопки
				if (tag === 'BUTTON' || tag === 'A' || tag === 'INPUT' ||
					role === 'button' || role === 'link' ||
					clickable !== null || hasPointer ||
					el.classList.contains('button') || el.classList.contains('btn')) {
					return true;
				}
				
				// Кнопки добавления в корзину (часто это div или span)
				if (className.includes('add') || className.includes('cart') || className.includes('basket') ||
					id.includes('add') || id.includes('cart') || id.includes('basket') ||
					className.includes('plus') || className.includes('increment') ||
					el.getAttribute('data-testid')?.toLowerCase().includes('add') ||
					el.getAttribute('data-qa')?.toLowerCase().includes('add') ||
					el.getAttribute('aria-label')?.toLowerCase().includes('добавить') ||
					el.getAttribute('aria-label')?.toLowerCase().includes('add')) {
					return true;
				}
				
				// Элементы с обработчиками событий
				if (el.addEventListener || el.onmousedown || el.ontouchstart) {
					return true;
				}
				
				return false;
			}
			
			function getDirectText(el) {
				return Array.from(el.childNodes)
					.filter(node => node.nodeType === Node.TEXT_NODE)
					.map(node => node.textContent)
					.join(' ')
					.trim();
			}
			
			// Функция для получения текста элемента, включая иконки и символы
			function getElementText(el) {
				// Обычный текст
				let text = (el.innerText || el.textContent || '').trim();
				
				// Если текста нет, пробуем aria-label, title
				if (!text) {
					text = (el.getAttribute('aria-label') || el.getAttribute('title') || '').trim();
				}
				
				// Если текста нет, ищем символы (+, -, ×) в тексте
				if (!text) {
					const symbols = el.textContent.match(/[+×−−−]/);
					if (symbols && symbols.length > 0) {
						text = symbols[0];
					}
				}
				
				// Если текста нет, ищем символ "+" в SVG
				if (!text) {
					const svg = el.querySelector('svg');
					if (svg) {
						// Ищем текст в SVG
						const svgText = svg.textContent || svg.getAttribute('aria-label') || '';
						if (svgText && svgText.includes('+')) {
							text = '+';
						}
						// Ищем path с признаками плюса
						const paths = svg.querySelectorAll('path, line, circle, rect');
						paths.forEach(path => {
							const d = path.getAttribute('d') || '';
							// Простая эвристика: если есть вертикальные и горизонтальные линии, это может быть плюс
							if (d.includes('M') && d.includes('L') && !text) {
								// Проверяем, есть ли в родительском элементе текст "+"
								const parentText = (el.textContent || '').trim();
								if (parentText === '+' || parentText.includes('+')) {
									text = '+';
								}
							}
						});
					}
				}
				
				// Если текста нет, ищем по классам/ID для кнопок добавления
				if (!text) {
					const className = (typeof el.className === 'string' ? el.className : (el.className ? el.className.toString() : '')).toLowerCase();
					const id = (el.id || '').toLowerCase();
					const dataTestid = (el.getAttribute('data-testid') || '').toLowerCase();
					const dataQa = (el.getAttribute('data-qa') || '').toLowerCase();
					
					if (className.includes('add') || className.includes('cart') || className.includes('basket') ||
						id.includes('add') || id.includes('cart') || id.includes('basket') ||
						className.includes('plus') || className.includes('increment') ||
						dataTestid.includes('add') || dataQa.includes('add')) {
						text = '+';
					}
				}
				
				// Проверяем псевдоэлементы (::before, ::after) через computed styles
				if (!text) {
					const style = window.getComputedStyle(el, '::before');
					const beforeContent = style.content;
					if (beforeContent && (beforeContent.includes('+') || beforeContent === '"+"' || beforeContent === "'+'")) {
						text = '+';
					}
					if (!text) {
						const afterStyle = window.getComputedStyle(el, '::after');
						const afterContent = afterStyle.content;
						if (afterContent && (afterContent.includes('+') || afterContent === '"+"' || afterContent === "'+'")) {
							text = '+';
						}
					}
				}
				
				return text;
			}
			
			const allElements = Array.from(document.querySelectorAll('*'));
			
			let target = allElements.find(el => {
				if (!isVisible(el) || !isClickable(el)) return false;
				const text = getElementText(el);
				return text.toLowerCase() === searchLower;
			});
			
			// Поиск по частичному совпадению с учетом иконок
			if (!target) {
				target = allElements.find(el => {
					if (!isVisible(el) || !isClickable(el)) return false;
					const text = getElementText(el);
					return text.toLowerCase().includes(searchLower) || searchLower.includes(text.toLowerCase());
				});
			}
			
			// Поиск кнопок добавления в корзину по специальным признакам
			if (!target && (searchLower.includes('добавить') || searchLower.includes('корзин') || searchLower === '+' || searchLower.includes('add') || searchLower.includes('cart'))) {
				target = allElements.find(el => {
					if (!isVisible(el) || !isClickable(el)) return false;
					const className = (typeof el.className === 'string' ? el.className : (el.className ? el.className.toString() : '')).toLowerCase();
					const id = (el.id || '').toLowerCase();
					const ariaLabel = (el.getAttribute('aria-label') || '').toLowerCase();
					const text = getElementText(el).toLowerCase();
					
					// Ищем кнопки с признаками добавления в корзину
					return className.includes('add') || className.includes('cart') || className.includes('basket') ||
					       id.includes('add') || id.includes('cart') || id.includes('basket') ||
					       ariaLabel.includes('добавить') || ariaLabel.includes('корзин') ||
					       ariaLabel.includes('add') || ariaLabel.includes('cart') ||
					       text === '+' || text.includes('добавить') || text.includes('корзин');
				});
			}
			
			// Поиск кнопок с символом "+" - расширенный поиск
			if (!target && (searchLower === '+' || searchLower.includes('плюс') || searchLower.includes('добавить'))) {
				// Сначала ищем точное совпадение
				target = allElements.find(el => {
					if (!isVisible(el)) return false;
					if (!isClickable(el)) {
						// Для кнопок добавления разрешаем даже если isClickable строгий
						const className = (typeof el.className === 'string' ? el.className : (el.className ? el.className.toString() : '')).toLowerCase();
						const id = (el.id || '').toLowerCase();
						if (!(className.includes('add') || className.includes('cart') || className.includes('basket') ||
							id.includes('add') || id.includes('cart') || id.includes('basket'))) {
							return false;
						}
					}
					const text = getElementText(el);
					return text === '+' || text.includes('+');
				});
				
				// Если не нашли, ищем по визуальным признакам (белый круг с плюсом)
				if (!target) {
					target = allElements.find(el => {
						if (!isVisible(el)) return false;
						const style = window.getComputedStyle(el);
						const bgColor = style.backgroundColor;
						const borderRadius = style.borderRadius;
						const width = el.offsetWidth;
						const height = el.offsetHeight;
						
						// Ищем круглые белые кнопки (типичные для кнопок добавления)
						const isRound = borderRadius && (parseFloat(borderRadius) >= width / 2 || borderRadius.includes('50%'));
						const isWhite = bgColor && (bgColor.includes('255, 255, 255') || bgColor.includes('rgb(255, 255, 255)') || bgColor === 'white');
						
						if ((isRound || width === height) && width > 20 && width < 100) {
							const text = getElementText(el);
							if (text === '+' || text.includes('+') || el.textContent.includes('+')) {
								return true;
							}
							// Проверяем наличие SVG с плюсом
							const svg = el.querySelector('svg');
							if (svg) {
								return true; // Если есть SVG в круглой кнопке, вероятно это кнопка добавления
							}
						}
						return false;
					});
				}
				
				// Если все еще не нашли, ищем любую кнопку с символом "+" в карточке товара
				if (!target) {
					// Ищем карточки товаров
					const productCards = Array.from(document.querySelectorAll('[class*="card"], [class*="product"], [class*="item"]'));
					for (const card of productCards) {
						if (!target) {
							const plusButton = Array.from(card.querySelectorAll('*')).find(el => {
								if (!isVisible(el)) return false;
								const text = getElementText(el);
								return (text === '+' || text.includes('+')) && 
								       (isClickable(el) || 
								        (typeof el.className === 'string' ? el.className : (el.className ? el.className.toString() : '')).toLowerCase().includes('add'));
							});
							if (plusButton) {
								target = plusButton;
								break;
							}
						}
					}
				}
			}
			
			// Резервный поиск - любая видимая кнопка
			if (!target) {
				target = allElements.find(el => {
					if (!isVisible(el)) return false;
					const text = getElementText(el);
					return text.toLowerCase() === searchLower;
				});
			}
			
			if (!target) {
				target = allElements.find(el => {
					if (!isVisible(el)) return false;
					const text = getElementText(el);
					return text.toLowerCase().includes(searchLower);
				});
			}
			
			if (target) {
				try {
					target.click();
				} catch (e) {
					const event = new MouseEvent('click', {
						bubbles: true,
						cancelable: true,
						view: window
					});
					target.dispatchEvent(event);
				}
				return true;
			}
			
			return false;
		})()
	`, escapedText)

	var clicked bool
	err := chromedp.Run(ctx,
		chromedp.Evaluate(script, &clicked),
		chromedp.Sleep(1*time.Second),
	)

	if err != nil {
		return fmt.Errorf("failed to click by text: %w", err)
	}

	if !clicked {
		return fmt.Errorf("element with text '%s' not found", text)
	}

	return nil
}

func (b *Browser) FillInput(selector, value string) error {
	// Проверяем, не отменен ли контекст браузера
	select {
	case <-b.ctx.Done():
		return fmt.Errorf("browser context was canceled - браузер недоступен")
	default:
	}

	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()

	return chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Clear(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, value, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
	)
}

func (b *Browser) FillInputByPlaceholder(placeholder, value string) error {
	// Проверяем, не отменен ли контекст браузера
	select {
	case <-b.ctx.Done():
		return fmt.Errorf("browser context was canceled - браузер недоступен")
	default:
	}

	ctx, cancel := context.WithTimeout(b.ctx, 20*time.Second)
	defer cancel()

	// Ждем загрузки страницы и появления динамического контента
	// Если ищем поле сопроводительного письма, ждем дольше, так как оно появляется после клика
	isCoverLetterField := strings.Contains(strings.ToLower(placeholder), "сопроводительное") || 
	                      strings.Contains(strings.ToLower(placeholder), "письм") ||
	                      len(value) > 50 // Длинный текст обычно означает сопроводительное письмо
	
	// Для полей поиска на сайтах доставки еды (самокат, яндекс.еда) также нужно подождать
	isSearchField := strings.Contains(strings.ToLower(placeholder), "искать") || 
	                 strings.Contains(strings.ToLower(placeholder), "search") ||
	                 strings.Contains(strings.ToLower(placeholder), "поиск")
	
	waitTime := 2 * time.Second
	if isCoverLetterField {
		waitTime = 3 * time.Second // Дольше ждем для динамически появляющихся полей
	} else if isSearchField {
		waitTime = 3 * time.Second // Для полей поиска тоже ждем дольше, так как они могут загружаться динамически
	}
	
	_ = chromedp.Run(ctx,
		chromedp.Sleep(waitTime), // Ждем загрузки динамического контента
		chromedp.Evaluate(`document.readyState === 'complete'`, nil),
	)
	
	// Для полей сопроводительного письма делаем дополнительное ожидание появления textarea
	if isCoverLetterField {
		_ = chromedp.Run(ctx,
			chromedp.Sleep(1*time.Second),
			chromedp.Evaluate(`
				(function() {
					// Ждем появления textarea на странице
					const maxWait = 3000; // 3 секунды максимум
					const startTime = Date.now();
					while (Date.now() - startTime < maxWait) {
						const textareas = Array.from(document.querySelectorAll('textarea'));
						const visibleTextareas = textareas.filter(ta => {
							const style = window.getComputedStyle(ta);
							return style.display !== 'none' && 
							       style.visibility !== 'hidden' && 
							       style.opacity !== '0' &&
							       ta.offsetWidth > 0 && 
							       ta.offsetHeight > 0;
						});
						if (visibleTextareas.length > 0) {
							return true;
						}
						// Небольшая задержка перед следующей проверкой
						const endTime = Date.now() + 100;
						while (Date.now() < endTime) {}
					}
					return false;
				})()
			`, nil),
		)
	}

	escapedPlaceholder := escapeJSString(placeholder)
	escapedValue := escapeJSString(value)
	
	// КРИТИЧЕСКИ ВАЖНО: Если placeholder очень длинный (>100 символов), это скорее всего сам текст письма
	// В этом случае нужно искать textarea, а не input, и исключать поисковые поля
	isLongText := len(placeholder) > 100 || len(value) > 100

	script := fmt.Sprintf(`
		(function() {
			const searchText = '%s'.toLowerCase();
			const searchWords = searchText.split(/\s+/).filter(w => w.length > 2); // Разбиваем на слова
			const isLongText = %t; // Передаем флаг из Go
			
			function isVisible(el) {
				if (!el) return false;
				const style = window.getComputedStyle(el);
				return style.display !== 'none' && 
					   style.visibility !== 'hidden' && 
					   style.opacity !== '0' &&
					   el.offsetWidth > 0 && 
					   el.offsetHeight > 0;
			}
			
			function matchesSearch(el, searchText, searchWords) {
				const placeholder = (el.placeholder || '').toLowerCase();
				const name = (el.name || '').toLowerCase();
				const id = (el.id || '').toLowerCase();
				const ariaLabel = (el.getAttribute('aria-label') || '').toLowerCase();
				const title = (el.getAttribute('title') || '').toLowerCase();
				const className = (typeof el.className === 'string' ? el.className : (el.className ? el.className.toString() : '')).toLowerCase();
				const type = (el.type || '').toLowerCase();
				const role = (el.getAttribute('role') || '').toLowerCase();
				
				// Точное совпадение
				if (placeholder === searchText || name === searchText || id === searchText) {
					return true;
				}
				
				// Частичное совпадение
				if (placeholder.includes(searchText) || 
					name.includes(searchText) || 
					id.includes(searchText) || 
					ariaLabel.includes(searchText) ||
					title.includes(searchText)) {
					return true;
				}
				
				// Поиск по словам (если ищем "что вы ищете", найдем "искать")
				if (searchWords.length > 0) {
					for (let word of searchWords) {
						if (placeholder.includes(word) || 
							name.includes(word) || 
							ariaLabel.includes(word) ||
							title.includes(word)) {
							return true;
						}
					}
				}
				
				// Специальная логика для полей поиска
				if (type === 'search' || 
					role === 'searchbox' ||
					className.includes('search') ||
					id.includes('search') ||
					name.includes('search') ||
					placeholder.includes('искать') ||
					placeholder.includes('search') ||
					ariaLabel.includes('искать') ||
					ariaLabel.includes('search')) {
					return true;
				}
				
				return false;
			}
			
			// Ищем все input и textarea
			const allInputs = Array.from(document.querySelectorAll('input, textarea'));
			
			// Функция для проверки, является ли поле видимым и доступным
			function isValidInput(i) {
				if (!i) return false;
				const type = (i.type || '').toLowerCase();
				// Пропускаем скрытые поля
				if (type === 'hidden') return false;
				// Проверяем видимость
				return isVisible(i);
			}
			
			// Специальная логика для полей сопроводительного письма
			// Определяем, что это поле для письма по длине value (передается через escapedValue, но мы можем проверить searchText)
			// Если placeholder очень длинный (>100 символов), это скорее всего сам текст письма, а не поиск поля
			const isCoverLetterSearch = searchText.includes('сопроводительное') || 
			                            searchText.includes('письм') || 
			                            searchText.includes('cover') || 
			                            searchText.includes('letter') ||
			                            searchText.length > 100 || // Длинный текст обычно означает, что это само письмо
			                            isLongText; // Флаг из Go (если placeholder или value > 100 символов)
			
			// КРИТИЧЕСКИ ВАЖНО: Если это сопроводительное письмо, НЕ ищем в поисковых полях!
			// Сначала ищем textarea (приоритет для длинных текстов)
			let target = null;
			
			if (isCoverLetterSearch) {
				// Для сопроводительного письма сначала ищем textarea с признаками поля для письма
				target = allInputs.find(i => {
					if (!isValidInput(i)) return false;
					// ИСКЛЮЧАЕМ поисковые поля
					const type = (i.type || '').toLowerCase();
					const className = (typeof i.className === 'string' ? i.className : (i.className ? i.className.toString() : '')).toLowerCase();
					const id = (i.id || '').toLowerCase();
					const name = (i.name || '').toLowerCase();
					
					// Пропускаем поисковые поля
					if (type === 'search' || 
					    className.includes('search') || 
					    id.includes('search') || 
					    name.includes('search') ||
					    className.includes('header') ||
					    className.includes('nav')) {
						return false;
					}
					
					// Ищем textarea (обычно используется для длинных текстов)
					if (i.tagName !== 'TEXTAREA') return false;
					
					const placeholder = (i.placeholder || '').toLowerCase();
					const ariaLabel = (i.getAttribute('aria-label') || '').toLowerCase();
					
					// Ищем по ключевым словам для сопроводительного письма
					return placeholder.includes('сопроводительное') || placeholder.includes('письм') || 
					       placeholder.includes('cover') || placeholder.includes('letter') ||
					       ariaLabel.includes('сопроводительное') || ariaLabel.includes('письм') ||
					       ariaLabel.includes('cover') || ariaLabel.includes('letter');
				});
				
				// Если не нашли по ключевым словам, ищем самый большой textarea (но НЕ в header/nav)
				if (!target) {
					let largestTextarea = null;
					let largestSize = 0;
					allInputs.forEach(i => {
						if (i.tagName === 'TEXTAREA' && isValidInput(i)) {
							// ИСКЛЮЧАЕМ поля в header/nav (обычно там поиск)
							const parent = i.closest('header, nav, [class*="header"], [class*="nav"]');
							if (parent) return;
							
							const className = (typeof i.className === 'string' ? i.className : (i.className ? i.className.toString() : '')).toLowerCase();
							const id = (i.id || '').toLowerCase();
							// Пропускаем поисковые поля
							if (className.includes('search') || id.includes('search')) return;
							
							const size = i.offsetWidth * i.offsetHeight;
							// Для сопроводительного письма обычно нужен большой textarea (минимум 3000px^2)
							if (size > largestSize && size > 3000) {
								largestSize = size;
								largestTextarea = i;
							}
						}
					});
					if (largestTextarea) {
						target = largestTextarea;
					}
				}
			}
			
			// Если не нашли через специальную логику, ищем обычным способом (но исключаем поисковые поля для длинных текстов)
			if (!target) {
				target = allInputs.find(i => {
					if (!isValidInput(i)) return false;
					
					// Если это длинный текст, исключаем поисковые поля
					if (isCoverLetterSearch || searchText.length > 100) {
						const type = (i.type || '').toLowerCase();
						const className = (typeof i.className === 'string' ? i.className : (i.className ? i.className.toString() : '')).toLowerCase();
						const id = (i.id || '').toLowerCase();
						const name = (i.name || '').toLowerCase();
						const parent = i.closest('header, nav, [class*="header"], [class*="nav"]');
						
						// Пропускаем поисковые поля
						if (type === 'search' || 
						    className.includes('search') || 
						    id.includes('search') || 
						    name.includes('search') ||
						    parent !== null) {
							return false;
						}
					}
					
					return matchesSearch(i, searchText, searchWords);
				});
			}
			
			// Если не нашли, ищем поле поиска (type="search" или role="searchbox")
			if (!target) {
				target = allInputs.find(i => {
					if (!isValidInput(i)) return false;
					const type = (i.type || '').toLowerCase();
					const role = (i.getAttribute('role') || '').toLowerCase();
					return type === 'search' || role === 'searchbox';
				});
			}
			
			// Если все еще не нашли, ищем по классам/ID содержащим "search"
			if (!target) {
				target = allInputs.find(i => {
					if (!isValidInput(i)) return false;
					const className = (typeof i.className === 'string' ? i.className : (i.className ? i.className.toString() : '')).toLowerCase();
					const id = (i.id || '').toLowerCase();
					const name = (i.name || '').toLowerCase();
					return className.includes('search') || id.includes('search') || name.includes('search');
				});
			}
			
			// Если не нашли, ищем поле с placeholder содержащим "искать" или "search"
			if (!target) {
				target = allInputs.find(i => {
					if (!isValidInput(i)) return false;
					const placeholder = (i.placeholder || '').toLowerCase();
					return placeholder.includes('искать') || placeholder.includes('search');
				});
			}
			
			// Если не нашли, ищем поле рядом с иконкой поиска (визуальный признак)
			if (!target) {
				const searchIcons = Array.from(document.querySelectorAll('svg, [class*="search"], [class*="magnify"], [aria-label*="search"], [aria-label*="искать"]'));
				searchIcons.forEach(icon => {
					if (!target) {
						// Ищем input в том же родительском элементе или рядом
						const parent = icon.closest('form, div, section, header, nav');
						if (parent) {
							const nearbyInput = parent.querySelector('input[type="text"], input[type="search"], input:not([type="hidden"]):not([type="submit"]):not([type="button"])');
							if (nearbyInput && isValidInput(nearbyInput)) {
								target = nearbyInput;
							}
						}
					}
				});
			}
			
			// Если не нашли, ищем любое видимое текстовое поле (для запросов с "искать"/"search")
			// НО только если это НЕ сопроводительное письмо
			if (!target && !isCoverLetterSearch && (searchText.includes('искать') || searchText.includes('search') || searchText.includes('поиск'))) {
				// Сначала ищем поле с максимальной шириной (обычно это главное поле поиска)
				let largestSearchInput = null;
				let largestSize = 0;
				allInputs.forEach(i => {
					if (isValidInput(i)) {
						const type = (i.type || '').toLowerCase();
						if (type === 'text' || type === 'search' || type === '' || type === 'email') {
							const size = i.offsetWidth * i.offsetHeight;
							if (size > largestSize && size > 200) { // Минимальный размер для поля поиска
								largestSize = size;
								largestSearchInput = i;
							}
						}
					}
				});
				if (largestSearchInput) {
					target = largestSearchInput;
				} else {
					// Если не нашли большое поле, берем первое подходящее
					target = allInputs.find(i => {
						if (!isValidInput(i)) return false;
						const type = (i.type || '').toLowerCase();
						return type === 'text' || type === 'search' || type === '' || type === 'email';
					});
				}
			}
			
			// Если не нашли, ищем input в header или nav (обычно там поле поиска)
			if (!target) {
				const header = document.querySelector('header, nav, [class*="header"], [class*="nav"], [class*="search"]');
				if (header) {
					const headerInputs = Array.from(header.querySelectorAll('input'));
					target = headerInputs.find(i => {
						if (!isValidInput(i)) return false;
						const type = (i.type || '').toLowerCase();
						return type === 'text' || type === 'search' || type === '' || type === 'email';
					});
				}
			}
			
			// Поиск по data-атрибутам (data-qa, data-testid и т.д.)
			if (!target) {
				target = allInputs.find(i => {
					if (!isValidInput(i)) return false;
					const dataQa = (i.getAttribute('data-qa') || '').toLowerCase();
					const dataTestid = (i.getAttribute('data-testid') || '').toLowerCase();
					const dataId = (i.getAttribute('data-id') || '').toLowerCase();
					const dataCy = (i.getAttribute('data-cy') || '').toLowerCase();
					return dataQa.includes('search') || dataQa.includes('искать') || dataQa.includes('query') ||
					       dataTestid.includes('search') || dataTestid.includes('искать') || dataTestid.includes('query') ||
					       dataId.includes('search') || dataId.includes('искать') || dataId.includes('query') ||
					       dataCy.includes('search') || dataCy.includes('искать') || dataCy.includes('query');
				});
			}
			
			// Поиск по inputmode (для мобильных полей поиска)
			if (!target) {
				target = allInputs.find(i => {
					if (!isValidInput(i)) return false;
					const inputmode = (i.getAttribute('inputmode') || '').toLowerCase();
					return inputmode === 'search';
				});
			}
			
			// Поиск по enterkeyhint (для полей поиска на мобильных)
			if (!target) {
				target = allInputs.find(i => {
					if (!isValidInput(i)) return false;
					const enterkeyhint = (i.getAttribute('enterkeyhint') || '').toLowerCase();
					return enterkeyhint === 'search';
				});
			}
			
			// Поиск по autocomplete атрибутам
			if (!target) {
				target = allInputs.find(i => {
					if (!isValidInput(i)) return false;
					const autocomplete = (i.getAttribute('autocomplete') || '').toLowerCase();
					return autocomplete.includes('search') || autocomplete === 'off';
				});
			}
			
			// Поиск поля рядом с текстом "поиск", "искать", "search"
			if (!target) {
				const searchTextElements = Array.from(document.querySelectorAll('*')).filter(el => {
					const text = (el.innerText || el.textContent || '').toLowerCase();
					return text.includes('поиск') || text.includes('искать') || text.includes('search');
				});
				for (const textEl of searchTextElements) {
					if (!target) {
						// Ищем input в том же родительском элементе
						const parent = textEl.closest('form, div, section, header, nav, [class*="search"]');
						if (parent) {
							const nearbyInput = parent.querySelector('input[type="text"], input[type="search"], input:not([type="hidden"]):not([type="submit"]):not([type="button"])');
							if (nearbyInput && isValidInput(nearbyInput)) {
								target = nearbyInput;
								break;
							}
						}
					}
				}
			}
			
			// Поиск в формах
			if (!target) {
				const forms = Array.from(document.querySelectorAll('form'));
				for (const form of forms) {
					if (!target) {
						const formInputs = Array.from(form.querySelectorAll('input'));
						target = formInputs.find(i => {
							if (!isValidInput(i)) return false;
							const type = (i.type || '').toLowerCase();
							return type === 'text' || type === 'search' || type === '' || type === 'email';
						});
					}
				}
			}
			
			// Последняя попытка - ищем самое большое видимое текстовое поле (обычно это поле поиска)
			if (!target) {
				let largestInput = null;
				let largestSize = 0;
				allInputs.forEach(i => {
					if (isValidInput(i)) {
						const type = (i.type || '').toLowerCase();
						if (type === 'text' || type === 'search' || type === '' || type === 'email') {
							const size = i.offsetWidth * i.offsetHeight;
							if (size > largestSize && size > 500) { // Уменьшили минимальный размер до 500
								largestSize = size;
								largestInput = i;
							}
						}
					}
				});
				if (largestInput) {
					target = largestInput;
				}
			}
			
			// Абсолютно последняя попытка - первое видимое текстовое поле
			// НО только если это НЕ сопроводительное письмо (для письма уже искали textarea)
			if (!target && !isCoverLetterSearch && (searchText.includes('искать') || searchText.includes('search') || searchText.includes('поиск'))) {
				target = allInputs.find(i => {
					if (!isValidInput(i)) return false;
					const type = (i.type || '').toLowerCase();
					return (type === 'text' || type === 'search' || type === '' || type === 'email') && 
					       i.offsetWidth > 50 && i.offsetHeight > 20; // Минимальные размеры
				});
			}
			
			if (target) {
				// Прокручиваем к полю, если оно не в видимой области
				target.scrollIntoView({ behavior: 'smooth', block: 'center' });
				// Небольшая задержка для прокрутки
				setTimeout(() => {
					target.focus();
					target.value = '%s';
					target.dispatchEvent(new Event('input', { bubbles: true }));
					target.dispatchEvent(new Event('change', { bubbles: true }));
					target.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', keyCode: 13, bubbles: true }));
				}, 200);
				return true;
			}
			return false;
		})()
	`, escapedPlaceholder, isLongText, escapedValue)

	var filled bool
	err := chromedp.Run(ctx,
		chromedp.Evaluate(script, &filled),
		chromedp.Sleep(1*time.Second), // Увеличена задержка для обработки событий
	)

	if err != nil {
		return fmt.Errorf("failed to fill input: %w", err)
	}

	if !filled {
		// Попробуем еще раз с более агрессивным поиском
		time.Sleep(1 * time.Second)
		fallbackScript := fmt.Sprintf(`
			(function() {
				function isVisible(el) {
					if (!el) return false;
					const style = window.getComputedStyle(el);
					return style.display !== 'none' && 
						   style.visibility !== 'hidden' && 
						   style.opacity !== '0' &&
						   el.offsetWidth > 0 && 
						   el.offsetHeight > 0;
				}
				
				// Ищем любое видимое текстовое поле
				const inputs = Array.from(document.querySelectorAll('input, textarea'));
				let target = inputs.find(i => {
					if (!i) return false;
					const type = (i.type || '').toLowerCase();
					if (type === 'hidden' || type === 'submit' || type === 'button' || type === 'checkbox' || type === 'radio') return false;
					if (!isVisible(i)) return false;
					return type === 'text' || type === 'search' || type === '' || type === 'email' || i.tagName === 'TEXTAREA';
				});
				
				// Если не нашли, ищем в header/nav
				if (!target) {
					const header = document.querySelector('header, nav, [class*="header"], [class*="nav"]');
					if (header) {
						const headerInputs = Array.from(header.querySelectorAll('input'));
						target = headerInputs.find(i => {
							if (!i) return false;
							const type = (i.type || '').toLowerCase();
							if (type === 'hidden' || type === 'submit' || type === 'button') return false;
							return isVisible(i) && (type === 'text' || type === 'search' || type === '');
						});
					}
				}
				
				// Если все еще не нашли, берем первое видимое текстовое поле
				if (!target) {
					target = inputs.find(i => {
						if (!i) return false;
						const type = (i.type || '').toLowerCase();
						if (type === 'hidden' || type === 'submit' || type === 'button' || type === 'checkbox' || type === 'radio') return false;
						return isVisible(i) && (type === 'text' || type === 'search' || type === '' || type === 'email') &&
						       i.offsetWidth > 50 && i.offsetHeight > 20;
					});
				}
				
				if (target) {
					target.scrollIntoView({ behavior: 'smooth', block: 'center' });
					setTimeout(() => {
						target.focus();
						target.value = '%s';
						target.dispatchEvent(new Event('input', { bubbles: true }));
						target.dispatchEvent(new Event('change', { bubbles: true }));
						target.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', keyCode: 13, bubbles: true }));
					}, 300);
					return true;
				}
				return false;
			})()
		`, escapedValue)
		
		err2 := chromedp.Run(ctx,
			chromedp.Evaluate(fallbackScript, &filled),
			chromedp.Sleep(500*time.Millisecond),
		)
		
		if err2 == nil && filled {
			return nil
		}
		
		return fmt.Errorf("input field matching '%s' not found (tried placeholder, name, id, aria-label, search icons, header/nav, largest field)", placeholder)
	}

	return nil
}

func (b *Browser) WaitForElement(selector string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()

	return chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
	)
}

func (b *Browser) GetCurrentURL() (string, error) {
	// Проверяем, не отменен ли контекст браузера
	select {
	case <-b.ctx.Done():
		return "", fmt.Errorf("browser context was canceled - браузер недоступен")
	default:
	}

	// Увеличиваем таймаут и добавляем повторные попытки
	maxRetries := 2
	var url string
	var err error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
		
		err = chromedp.Run(ctx,
			chromedp.Evaluate("window.location.href", &url),
		)
		
		cancel()
		
		if err == nil {
			return url, nil
		}
		
		// Проверяем, не отменен ли контекст браузера
		select {
		case <-b.ctx.Done():
			return "", fmt.Errorf("browser context was canceled - браузер недоступен")
		default:
		}
		
		// Если это не последняя попытка, ждем перед повтором
		if attempt < maxRetries {
			time.Sleep(1 * time.Second)
			continue
		}
	}

	return url, fmt.Errorf("failed to get URL after %d attempts: %w", maxRetries, err)
}

func (b *Browser) Screenshot(filename string) error {
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()

	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.CaptureScreenshot(&buf),
	)

	if err != nil {
		return fmt.Errorf("failed to take screenshot: %w", err)
	}

	return os.WriteFile(filename, buf, 0644)
}

func (b *Browser) keepAliveLoop() {
	ticker := time.NewTicker(30 * time.Second) // Уменьшаем интервал для более частых проверок
	defer ticker.Stop()

	for {
		select {
		case <-b.keepAlive.Done():
			return
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			// Проверяем, что контекст еще активен
			select {
			case <-b.ctx.Done():
				return
			default:
			}
			
			ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
			var url string
			err := chromedp.Run(ctx,
				chromedp.Evaluate("window.location.href", &url),
			)
			cancel()
			
			// Не выходим при ошибках таймаута - это нормально, просто продолжаем
			if err != nil {
				if err == context.Canceled {
					return
				}
				// Игнорируем DeadlineExceeded - просто продолжаем работу
			}
			_ = url
		}
	}
}

func (b *Browser) Close() error {
	b.keepAliveCancel()
	b.cancel()
	b.allocCancel()
	return nil
}

type PageContent struct {
	URL      string      `json:"url"`
	Title    string      `json:"title"`
	Text     string      `json:"text"`
	Links    []Link      `json:"links"`
	Buttons  []Button    `json:"buttons"`
	Inputs   []Input     `json:"inputs"`
	Headings []Heading   `json:"headings"`
	Lists    [][]string  `json:"lists,omitempty"`   // списки -> элементы
	Tables   [][][]string `json:"tables,omitempty"` // таблицы -> строки -> ячейки
}

type Link struct {
	Text string `json:"text"`
	Href string `json:"href"`
}

type Button struct {
	Text string `json:"text"`
	Type string `json:"type"`
	Role string `json:"role,omitempty"`
}

type Input struct {
	Type        string `json:"type"`
	Placeholder string `json:"placeholder"`
	Name        string `json:"name"`
	ID          string `json:"id,omitempty"`
	Label       string `json:"label,omitempty"`
}

type Heading struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

func escapeJSString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
