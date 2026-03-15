package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/nats-io/nats.go"
)

// Config NATS配置
type Config struct {
	URL string
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		URL: "nats://localhost:4222",
	}
}

// Publisher 事件发布器
type Publisher struct {
	nc *nats.Conn
}

// NewPublisher 创建事件发布器
func NewPublisher(cfg *Config) (*Publisher, error) {
	nc, err := nats.Connect(cfg.URL,
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(5),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			klog.Warnf("NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			klog.Info("NATS reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	klog.Info("NATS publisher initialized successfully")
	return &Publisher{nc: nc}, nil
}

// Close 关闭连接
func (p *Publisher) Close() error {
	p.nc.Close()
	return nil
}

// Publish 发布事件
func (p *Publisher) Publish(ctx context.Context, subject string, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if err := p.nc.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	return nil
}

// PublishAsync 异步发布事件
func (p *Publisher) PublishAsync(subject string, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	go func() {
		if err := p.nc.Publish(subject, data); err != nil {
			klog.Errorf("failed to publish event to %s: %v", subject, err)
		}
	}()

	return nil
}

// 事件主题定义
const (
	SubjectOrderCreated    = "order.created"
	SubjectOrderPaid       = "order.paid"
	SubjectOrderCanceled   = "order.canceled"
	SubjectPaymentSuccess  = "payment.success"
	SubjectPaymentRefund   = "payment.refund"
	SubjectStockDecrease   = "stock.decrease"
	SubjectStockIncrease   = "stock.increase"
	SubjectUserBehavior    = "user.behavior"
)

// 基础事件结构
type BaseEvent struct {
	EventID   string    `json:"event_id"`
	EventType string    `json:"event_type"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
}

// OrderCreatedEvent 订单创建事件
type OrderCreatedEvent struct {
	BaseEvent
	OrderID     string  `json:"order_id"`
	UserID      uint64  `json:"user_id"`
	TotalAmount float64 `json:"total_amount"`
	ItemCount   int     `json:"item_count"`
	Currency    string  `json:"currency"`
	ProductIDs  []uint64 `json:"product_ids"`
}

// OrderPaidEvent 订单支付事件
type OrderPaidEvent struct {
	BaseEvent
	OrderID       string    `json:"order_id"`
	UserID        uint64    `json:"user_id"`
	PaymentID     string    `json:"payment_id"`
	Amount        float64   `json:"amount"`
	PaymentMethod string    `json:"payment_method"`
	PaidAt        time.Time `json:"paid_at"`
}

// StockChangeEvent 库存变化事件
type StockChangeEvent struct {
	BaseEvent
	ProductID   uint64 `json:"product_id"`
	ChangeType  string `json:"change_type"` // decrease, increase, reserve, cancel_reserve
	Quantity    int32  `json:"quantity"`
	BeforeStock int32  `json:"before_stock"`
	AfterStock  int32  `json:"after_stock"`
	OrderID     string `json:"order_id,omitempty"`
	UserID      uint64 `json:"user_id,omitempty"`
}

// UserBehaviorEvent 用户行为事件
type UserBehaviorEvent struct {
	BaseEvent
	UserID       uint64 `json:"user_id"`
	SessionID    string `json:"session_id"`
	ActionType   string `json:"action_type"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	DeviceType   string `json:"device_type"`
	Platform     string `json:"platform"`
}

// NewBaseEvent 创建基础事件
func NewBaseEvent(eventType, source string) BaseEvent {
	return BaseEvent{
		EventID:   fmt.Sprintf("%d", time.Now().UnixNano()),
		EventType: eventType,
		Timestamp: time.Now(),
		Source:    source,
	}
}
