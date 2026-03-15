package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/cloudwego/kitex/pkg/klog"
)

// Config ClickHouse配置
type Config struct {
	Addr     string
	Database string
	User     string
	Password string
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Addr:     "localhost:9000",
		Database: "gomall_analytics",
		User:     "default",
		Password: "gomall",
	}
}

// Client ClickHouse客户端
type Client struct {
	conn driver.Conn
}

// NewClient 创建ClickHouse客户端
func NewClient(cfg *Config) (*Client, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.User,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:      time.Second * 10,
		MaxOpenConns:     10,
		MaxIdleConns:     5,
		ConnMaxLifetime:  time.Hour,
		ConnOpenStrategy: clickhouse.ConnOpenInOrder,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to clickhouse: %w", err)
	}

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	klog.Info("ClickHouse client initialized successfully")
	return &Client{conn: conn}, nil
}

// Conn 获取原始连接
func (c *Client) Conn() driver.Conn {
	return c.conn
}

// Close 关闭连接
func (c *Client) Close() error {
	return c.conn.Close()
}

// InsertOrderAnalytics 插入订单分析数据
func (c *Client) InsertOrderAnalytics(ctx context.Context, data *OrderAnalytics) error {
	query := `
		INSERT INTO order_analytics (
			order_id, user_id, order_state, total_amount, item_count, currency,
			created_at, created_date, created_hour, created_weekday,
			country, state, city, product_ids, category_ids,
			payment_method, payment_status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	return c.conn.AsyncInsert(ctx, query, false,
		data.OrderID, data.UserID, data.OrderState, data.TotalAmount, data.ItemCount, data.Currency,
		data.CreatedAt, data.CreatedAt.Format("2006-01-02"), data.CreatedAt.Hour(), int(data.CreatedAt.Weekday()),
		data.Country, data.State, data.City, data.ProductIDs, data.CategoryIDs,
		data.PaymentMethod, data.PaymentStatus,
	)
}

// InsertProductSales 插入商品销量数据
func (c *Client) InsertProductSales(ctx context.Context, data *ProductSales) error {
	query := `
		INSERT INTO product_sales (
			product_id, product_name, category_id, category_name,
			stat_date, stat_hour, order_count, quantity_sold, revenue, unique_buyers
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	return c.conn.AsyncInsert(ctx, query, false,
		data.ProductID, data.ProductName, data.CategoryID, data.CategoryName,
		data.StatDate, data.StatHour, data.OrderCount, data.QuantitySold, data.Revenue, data.UniqueBuyers,
	)
}

// InsertUserBehavior 插入用户行为数据
func (c *Client) InsertUserBehavior(ctx context.Context, data *UserBehavior) error {
	query := `
		INSERT INTO user_behavior (
			user_id, session_id, action_type, resource_type, resource_id,
			action_time, action_date, device_type, platform
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	return c.conn.AsyncInsert(ctx, query, false,
		data.UserID, data.SessionID, data.ActionType, data.ResourceType, data.ResourceID,
		data.ActionTime, data.ActionTime.Format("2006-01-02"), data.DeviceType, data.Platform,
	)
}

// InsertInventoryChange 插入库存变化数据
func (c *Client) InsertInventoryChange(ctx context.Context, data *InventoryChange) error {
	query := `
		INSERT INTO inventory_changes (
			product_id, change_type, quantity, before_stock, after_stock,
			order_id, user_id, change_time, change_date
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	return c.conn.AsyncInsert(ctx, query, false,
		data.ProductID, data.ChangeType, data.Quantity, data.BeforeStock, data.AfterStock,
		data.OrderID, data.UserID, data.ChangeTime, data.ChangeTime.Format("2006-01-02"),
	)
}

// QuerySalesTrend 查询销售趋势
func (c *Client) QuerySalesTrend(ctx context.Context, startDate, endDate time.Time) ([]*SalesTrend, error) {
	query := `
		SELECT stat_date, stat_hour, total_orders, total_revenue, total_items_sold, unique_customers, avg_order_value
		FROM sales_trend
		WHERE stat_date >= ? AND stat_date <= ?
		ORDER BY stat_date, stat_hour
	`

	rows, err := c.conn.Query(ctx, query, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("failed to query sales trend: %w", err)
	}
	defer rows.Close()

	var results []*SalesTrend
	for rows.Next() {
		var trend SalesTrend
		if err := rows.Scan(
			&trend.StatDate, &trend.StatHour, &trend.TotalOrders,
			&trend.TotalRevenue, &trend.TotalItemsSold, &trend.UniqueCustomers, &trend.AvgOrderValue,
		); err != nil {
			return nil, fmt.Errorf("failed to scan sales trend: %w", err)
		}
		results = append(results, &trend)
	}

	return results, nil
}

// QueryTopProducts 查询热销商品
func (c *Client) QueryTopProducts(ctx context.Context, startDate time.Time, limit int) ([]*ProductSales, error) {
	query := `
		SELECT product_id, product_name, category_id, category_name,
			   sum(order_count) as order_count, sum(quantity_sold) as quantity_sold,
			   sum(revenue) as revenue, uniqMerge(unique_buyers) as unique_buyers
		FROM product_sales
		WHERE stat_date >= ?
		GROUP BY product_id, product_name, category_id, category_name
		ORDER BY revenue DESC
		LIMIT ?
	`

	rows, err := c.conn.Query(ctx, query, startDate.Format("2006-01-02"), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top products: %w", err)
	}
	defer rows.Close()

	var results []*ProductSales
	for rows.Next() {
		var ps ProductSales
		if err := rows.Scan(
			&ps.ProductID, &ps.ProductName, &ps.CategoryID, &ps.CategoryName,
			&ps.OrderCount, &ps.QuantitySold, &ps.Revenue, &ps.UniqueBuyers,
		); err != nil {
			return nil, fmt.Errorf("failed to scan product sales: %w", err)
		}
		results = append(results, &ps)
	}

	return results, nil
}

// 数据结构定义

type OrderAnalytics struct {
	OrderID       string
	UserID        uint64
	OrderState    string
	TotalAmount   float64
	ItemCount     uint32
	Currency      string
	CreatedAt     time.Time
	Country       string
	State         string
	City          string
	ProductIDs    []uint64
	CategoryIDs   []uint32
	PaymentMethod string
	PaymentStatus string
}

type ProductSales struct {
	ProductID     uint64
	ProductName   string
	CategoryID    uint32
	CategoryName  string
	StatDate      time.Time
	StatHour      uint8
	OrderCount    uint64
	QuantitySold  uint64
	Revenue       float64
	UniqueBuyers  uint64
}

type UserBehavior struct {
	UserID       uint64
	SessionID    string
	ActionType   string
	ResourceType string
	ResourceID   string
	ActionTime   time.Time
	DeviceType   string
	Platform     string
}

type InventoryChange struct {
	ProductID   uint64
	ChangeType  string
	Quantity    int32
	BeforeStock int32
	AfterStock  int32
	OrderID     string
	UserID      uint64
	ChangeTime  time.Time
}

type SalesTrend struct {
	StatDate        time.Time
	StatHour        uint8
	TotalOrders     uint64
	TotalRevenue    float64
	TotalItemsSold  uint64
	UniqueCustomers uint64
	AvgOrderValue   float64
}
