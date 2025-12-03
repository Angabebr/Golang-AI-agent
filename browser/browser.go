package browser

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

// Browser управляет браузером через chromedp
type Browser struct {
	ctx         context.Context
	cancel      context.CancelFunc
	allocCtx    context.Context
	allocCancel context.CancelFunc
}

// NewBrowser создает новый экземпляр браузера
func NewBrowser(userDataDir string, headless bool) (*Browser, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", headless),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("disable-dev-shm-usage", false),
		chromedp.Flag("no-sandbox", false),
		chromedp.UserDataDir(userDataDir),
		chromedp.WindowSize(1920, 1080),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(format string, v ...interface{}) {
		// Логирование можно включить при необходимости
	}))

	b := &Browser{
		ctx:         ctx,
		cancel:      cancel,
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
	}

	// Устанавливаем таймаут для инициализации
	ctx, timeout := context.WithTimeout(ctx, 30*time.Second)
	defer timeout()

	if err := chromedp.Run(ctx); err != nil {
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	return b, nil
}

// Navigate переходит на указанный URL
func (b *Browser) Navigate(url string) error {
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()

	return chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	)
}

// GetPageContent извлекает структурированную информацию о странице
func (b *Browser) GetPageContent() (*PageContent, error) {
	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
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

// ClickElement кликает на элемент по селектору
func (b *Browser) ClickElement(selector string) error {
	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	return chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second), // Даем время на загрузку
	)
}

// ClickByText кликает на элемент по тексту
func (b *Browser) ClickByText(text string) error {
	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	script := fmt.Sprintf(`
		(function() {
			const elements = Array.from(document.querySelectorAll('*'));
			const target = elements.find(el => {
				const elText = el.innerText || el.textContent || '';
				return elText.trim().toLowerCase().includes('%s'.toLowerCase()) && 
					   el.offsetParent !== null &&
					   (el.tagName === 'BUTTON' || el.tagName === 'A' || el.getAttribute('role') === 'button' || el.onclick !== null);
			});
			if (target) {
				target.click();
				return true;
			}
			return false;
		})()
	`, text)

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

// FillInput заполняет поле ввода
func (b *Browser) FillInput(selector, value string) error {
	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	return chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Clear(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, value, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
	)
}

// FillInputByPlaceholder заполняет поле по placeholder
func (b *Browser) FillInputByPlaceholder(placeholder, value string) error {
	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	script := fmt.Sprintf(`
		(function() {
			const inputs = Array.from(document.querySelectorAll('input, textarea'));
			const target = inputs.find(i => i.placeholder && i.placeholder.toLowerCase().includes('%s'.toLowerCase()) && i.offsetParent !== null);
			if (target) {
				target.value = '%s';
				target.dispatchEvent(new Event('input', { bubbles: true }));
				target.dispatchEvent(new Event('change', { bubbles: true }));
				return true;
			}
			return false;
		})()
	`, placeholder, value)

	var filled bool
	err := chromedp.Run(ctx,
		chromedp.Evaluate(script, &filled),
		chromedp.Sleep(500*time.Millisecond),
	)

	if err != nil {
		return fmt.Errorf("failed to fill input: %w", err)
	}

	if !filled {
		return fmt.Errorf("input with placeholder '%s' not found", placeholder)
	}

	return nil
}

// WaitForElement ждет появления элемента
func (b *Browser) WaitForElement(selector string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()

	return chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
	)
}

// GetCurrentURL возвращает текущий URL
func (b *Browser) GetCurrentURL() (string, error) {
	ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
	defer cancel()

	var url string
	err := chromedp.Run(ctx,
		chromedp.Evaluate("window.location.href", &url),
	)

	return url, err
}

// Screenshot делает скриншот страницы
func (b *Browser) Screenshot(filename string) error {
	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
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

// Close закрывает браузер
func (b *Browser) Close() error {
	b.cancel()
	b.allocCancel()
	return nil
}

// PageContent содержит структурированную информацию о странице
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
