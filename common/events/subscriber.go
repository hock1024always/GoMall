package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/nats-io/nats.go"

	"github.com/xvxiaoman8/gomall/common/clickhouse"
)

// Subscriber 事件订阅器
type Subscriber struct {
	nc        *nats.Conn
	ch        *clickhouse.Client
	subs      []*nats.Subscription
	mu        sync.Mutex
}

// NewSubscriber 创建事件订阅器
func NewSubscriber(cfg *Config, chClient *clickhouse.Client) (*Subscriber, error) {
	nc, err := nats.Connect(cfg.URL,
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(5),
		nats.DeliverAll(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &Subscriber{
		nc: nc,
		ch: chClient,
	}, nil
}

// Start 开始订阅事件
func (s *Subscriber) Start(ctx context.Context) error {
	// 订阅订单创建事件
	sub, err := s.nc.Subscribe(SubjectOrderCreated, s.handleOrderCreated)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectOrderCreated, err)
	}
	s.subs = append(s.subs, sub)

	// 订阅订单支付事件
	sub, err = s.nc.Subscribe(SubjectOrderPaid, s.handleOrderPaid)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectOrderPaid, err)
	}
	s.subs = append(s.subs, sub)

	// 订阅库存变化事件
	sub, err = s.nc.Subscribe(SubjectStockDecrease, s.handleStockChange)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectStockDecrease, err)
	}
	s.subs = append(s.subs, sub)

	sub, err = s.nc.Subscribe(SubjectStockIncrease, s.handleStockChange)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectStockIncrease, err)
	}
	s.subs = append(s.subs, sub)

	// 订阅用户行为事件
	sub, err = s.nc.Subscribe(SubjectUserBehavior, s.handleUserBehavior)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectUserBehavior, err)
	}
	s.subs = append(s.subs, sub)

	klog.Info("Event subscriber started")
	return nil
}

// Stop 停止订阅
func (s *Subscriber) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sub := range s.subs {
		if err := sub.Unsubscribe(); err != nil {
			klog.Warnf("failed to unsubscribe: %v", err)
		}
	}
	s.nc.Close()
	return nil
}

// handleOrderCreated 处理订单创建事件
func (s *Subscriber) handleOrderCreated(msg *nats.Msg) {
	var event OrderCreatedEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		klog.Errorf("failed to unmarshal order created event: %v", err)
		return
	}

	// 写入ClickHouse
	data := &clickhouse.OrderAnalytics{
		OrderID:     event.OrderID,
		UserID:      event.UserID,
		OrderState:  "placed",
		TotalAmount: event.TotalAmount,
		ItemCount:   uint32(event.ItemCount),
		Currency:    event.Currency,
		CreatedAt:   event.Timestamp,
		ProductIDs:  event.ProductIDs,
	}

	ctx := context.Background()
	if err := s.ch.InsertOrderAnalytics(ctx, data); err != nil {
		klog.Errorf("failed to insert order analytics: %v", err)
		return
	}

	// 确认消息
	msg.Ack()
}

// handleOrderPaid 处理订单支付事件
func (s *Subscriber) handleOrderPaid(msg *nats.Msg) {
	var event OrderPaidEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		klog.Errorf("failed to unmarshal order paid event: %v", err)
		return
	}

	// 更新订单状态（这里简化处理，实际应该更新ClickHouse中的记录）
	klog.Infof("Order paid: %s, payment: %s", event.OrderID, event.PaymentID)

	msg.Ack()
}

// handleStockChange 处理库存变化事件
func (s *Subscriber) handleStockChange(msg *nats.Msg) {
	var event StockChangeEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		klog.Errorf("failed to unmarshal stock change event: %v", err)
		return
	}

	// 写入ClickHouse
	data := &clickhouse.InventoryChange{
		ProductID:   event.ProductID,
		ChangeType:  event.ChangeType,
		Quantity:    event.Quantity,
		BeforeStock: event.BeforeStock,
		AfterStock:  event.AfterStock,
		OrderID:     event.OrderID,
		UserID:      event.UserID,
		ChangeTime:  event.Timestamp,
	}

	ctx := context.Background()
	if err := s.ch.InsertInventoryChange(ctx, data); err != nil {
		klog.Errorf("failed to insert inventory change: %v", err)
		return
	}

	msg.Ack()
}

// handleUserBehavior 处理用户行为事件
func (s *Subscriber) handleUserBehavior(msg *nats.Msg) {
	var event UserBehaviorEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		klog.Errorf("failed to unmarshal user behavior event: %v", err)
		return
	}

	// 写入ClickHouse
	data := &clickhouse.UserBehavior{
		UserID:       event.UserID,
		SessionID:    event.SessionID,
		ActionType:   event.ActionType,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		ActionTime:   event.Timestamp,
		DeviceType:   event.DeviceType,
		Platform:     event.Platform,
	}

	ctx := context.Background()
	if err := s.ch.InsertUserBehavior(ctx, data); err != nil {
		klog.Errorf("failed to insert user behavior: %v", err)
		return
	}

	msg.Ack()
}
