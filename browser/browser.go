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
		chromedp.Flag("disable-features", "VizDisplayCompositor"),
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
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()

	var content PageContent

	err := chromedp.Run(ctx,
		chromedp.Evaluate(`
		(function() {
			return {
				url: window.location.href,
				title: document.title,
				text: document.body.innerText.substring(0, 10000),
				links: Array.from(document.querySelectorAll('a')).slice(0, 50).map(a => ({
					text: a.innerText.trim(),
					href: a.href,
					visible: a.offsetParent !== null
				})).filter(l => l.visible && l.text),
				buttons: Array.from(document.querySelectorAll('button, [role="button"], input[type="submit"], input[type="button"]')).slice(0, 30).map(b => ({
					text: b.innerText || b.value || b.getAttribute('aria-label') || '',
					type: b.tagName.toLowerCase(),
					visible: b.offsetParent !== null,
					enabled: !b.disabled
				})).filter(b => b.visible && b.enabled && b.text),
				inputs: Array.from(document.querySelectorAll('input[type="text"], input[type="email"], input[type="password"], textarea')).slice(0, 20).map(i => ({
					type: i.type || 'text',
					placeholder: i.placeholder || '',
					name: i.name || '',
					visible: i.offsetParent !== null
				})).filter(i => i.visible),
				headings: Array.from(document.querySelectorAll('h1, h2, h3')).slice(0, 20).map(h => ({
					level: h.tagName,
					text: h.innerText.trim()
				})).filter(h => h.text)
			};
		})()
		`, &content),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to extract page content: %w", err)
	}

	return &content, nil
}

func (b *Browser) ClickElement(selector string) error {
	ctx, cancel := context.WithTimeout(b.ctx, 20*time.Second)
	defer cancel()

	return chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	)
}

func (b *Browser) ClickByText(text string) error {
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
				
				return tag === 'BUTTON' || 
					   tag === 'A' || 
					   tag === 'INPUT' ||
					   role === 'button' || 
					   role === 'link' ||
					   clickable !== null ||
					   hasPointer ||
					   el.classList.contains('button') ||
					   el.classList.contains('btn');
			}
			
			function getDirectText(el) {
				return Array.from(el.childNodes)
					.filter(node => node.nodeType === Node.TEXT_NODE)
					.map(node => node.textContent)
					.join(' ')
					.trim();
			}
			
			const allElements = Array.from(document.querySelectorAll('*'));
			
			let target = allElements.find(el => {
				if (!isVisible(el) || !isClickable(el)) return false;
				const text = (el.innerText || el.textContent || '').trim();
				return text.toLowerCase() === searchLower;
			});
			
			if (!target) {
				target = allElements.find(el => {
					if (!isVisible(el) || !isClickable(el)) return false;
					const text = (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim();
					return text.toLowerCase() === searchLower.replace(/\s+/g, ' ');
				});
			}
			
			if (!target) {
				target = allElements.find(el => {
					if (!isVisible(el) || !isClickable(el)) return false;
					const text = (el.innerText || el.textContent || '').toLowerCase().trim();
					return text.includes(searchLower);
				});
			}
			
			if (!target) {
				target = allElements.find(el => {
					if (!isVisible(el)) return false;
					const text = (el.innerText || el.textContent || '').trim();
					return text.toLowerCase() === searchLower;
				});
			}
			
			if (!target) {
				target = allElements.find(el => {
					if (!isVisible(el)) return false;
					const text = (el.innerText || el.textContent || '').toLowerCase().trim();
					return text.includes(searchLower);
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
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()

	escapedPlaceholder := escapeJSString(placeholder)
	escapedValue := escapeJSString(value)

	script := fmt.Sprintf(`
		(function() {
			const searchText = '%s'.toLowerCase();
			const inputs = Array.from(document.querySelectorAll('input, textarea'));
			
			const target = inputs.find(i => {
				if (i.offsetParent === null && i.type !== 'hidden') return false;
				
				const placeholder = (i.placeholder || '').toLowerCase();
				const name = (i.name || '').toLowerCase();
				const id = (i.id || '').toLowerCase();
				const ariaLabel = (i.getAttribute('aria-label') || '').toLowerCase();
				
				return placeholder.includes(searchText) || 
					   name.includes(searchText) || 
					   name === searchText ||
					   id.includes(searchText) || 
					   ariaLabel.includes(searchText);
			});
			
			if (target) {
				target.focus();
				target.value = '%s';
				target.dispatchEvent(new Event('input', { bubbles: true }));
				target.dispatchEvent(new Event('change', { bubbles: true }));
				target.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', keyCode: 13, bubbles: true }));
				return true;
			}
			return false;
		})()
	`, escapedPlaceholder, escapedValue)

	var filled bool
	err := chromedp.Run(ctx,
		chromedp.Evaluate(script, &filled),
		chromedp.Sleep(500*time.Millisecond),
	)

	if err != nil {
		return fmt.Errorf("failed to fill input: %w", err)
	}

	if !filled {
		return fmt.Errorf("input field matching '%s' not found (tried placeholder, name, id, aria-label)", placeholder)
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
	ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
	defer cancel()

	var url string
	err := chromedp.Run(ctx,
		chromedp.Evaluate("window.location.href", &url),
	)

	return url, err
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
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-b.keepAlive.Done():
			return
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(b.ctx, 2*time.Second)
			var url string
			err := chromedp.Run(ctx,
				chromedp.Evaluate("window.location.href", &url),
			)
			cancel()
			if err != nil {
				if err == context.Canceled || err == context.DeadlineExceeded {
					return
				}
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
	URL      string    `json:"url"`
	Title    string    `json:"title"`
	Text     string    `json:"text"`
	Links    []Link    `json:"links"`
	Buttons  []Button  `json:"buttons"`
	Inputs   []Input   `json:"inputs"`
	Headings []Heading `json:"headings"`
}

type Link struct {
	Text string `json:"text"`
	Href string `json:"href"`
}

type Button struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

type Input struct {
	Type        string `json:"type"`
	Placeholder string `json:"placeholder"`
	Name        string `json:"name"`
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
