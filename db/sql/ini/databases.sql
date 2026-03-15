 CREATE DATABASE IF NOT EXISTS `cart`  
    DEFAULT CHARACTER SET = 'utf8mb4';;

CREATE DATABASE IF NOT EXISTS `checkout`
    DEFAULT CHARACTER SET = 'utf8mb4';

CREATE DATABASE IF NOT EXISTS `order`
    DEFAULT CHARACTER SET = 'utf8mb4';

CREATE DATABASE IF NOT EXISTS `payment`
    DEFAULT CHARACTER SET = 'utf8mb4';

CREATE DATABASE IF NOT EXISTS `product`
    DEFAULT CHARACTER SET = 'utf8mb4';

CREATE DATABASE IF NOT EXISTS `user`
    DEFAULT CHARACTER SET = 'utf8mb4';

-- Casbin 规则表（用于权限管理）
CREATE TABLE IF NOT EXISTS `user`.`casbin_rule` (
    `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
    `ptype` varchar(100) DEFAULT '',
    `v0` varchar(100) DEFAULT '',
    `v1` varchar(100) DEFAULT '',
    `v2` varchar(100) DEFAULT '',
    `v3` varchar(100) DEFAULT '',
    `v4` varchar(100) DEFAULT '',
    `v5` varchar(100) DEFAULT '',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_casbin_rule` (`ptype`,`v0`,`v1`,`v2`,`v3`,`v4`,`v5`),
    KEY `idx_ptype` (`ptype`),
    KEY `idx_v0` (`v0`),
    KEY `idx_v1` (`v1`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;