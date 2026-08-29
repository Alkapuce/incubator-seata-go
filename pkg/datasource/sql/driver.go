/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/datasource"
	mysql2 "seata.apache.org/seata-go/v2/pkg/datasource/sql/datasource/mysql"
	postgres2 "seata.apache.org/seata-go/v2/pkg/datasource/sql/datasource/postgres"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/util"
	"seata.apache.org/seata-go/v2/pkg/protocol/branch"
	"seata.apache.org/seata-go/v2/pkg/util/log"
)

const (
	// SeataATMySQLDriver MySQL driver for AT mode
	SeataATMySQLDriver = "seata-at-mysql"
	// SeataATPostgresDriver PostgreSQL driver for AT mode
	SeataATPostgresDriver = "seata-at-postgres"
	// SeataXAMySQLDriver MySQL driver for XA mode
	SeataXAMySQLDriver = "seata-xa-mysql"
	// SeataXAMariaDBDriver MariaDB driver for XA mode
	SeataXAMariaDBDriver = "seata-xa-mariadb"
	// SeataXAPostgresDriver PostgreSQL driver for XA mode
	SeataXAPostgresDriver = "seata-xa-postgres"
)

type driverDescriptor struct {
	dbType            types.DBType
	target            driver.Driver
	targetName        string
	parseDBName       func(dsn string) (string, error)
	newTableMetaCache func(db *sql.DB, dbName string) datasource.TableMetaCache
}

// SeataDriverDescriptor describes a database driver that can be wrapped by a
// Seata SQL proxy driver. It lets vendor integrations register an XA-capable
// driver without adding the vendor driver as a direct dependency of Seata Go.
type SeataDriverDescriptor struct {
	DBType            types.DBType
	Target            driver.Driver
	TargetName        string
	ParseDBName       func(dsn string) (string, error)
	NewTableMetaCache func(db *sql.DB, dbName string) datasource.TableMetaCache
}

func (d SeataDriverDescriptor) internal() (driverDescriptor, error) {
	if d.DBType <= 0 || d.DBType == types.DBTypeUnknown {
		return driverDescriptor{}, errors.New("db type is required")
	}
	if d.Target == nil {
		return driverDescriptor{}, errors.New("target driver is required")
	}
	if d.ParseDBName == nil {
		return driverDescriptor{}, errors.New("parse db name function is required")
	}
	targetName := d.TargetName
	if targetName == "" {
		targetName = d.DBType.String()
	}
	return driverDescriptor{
		dbType:            d.DBType,
		target:            d.Target,
		targetName:        targetName,
		parseDBName:       d.ParseDBName,
		newTableMetaCache: d.NewTableMetaCache,
	}, nil
}

// RegisterSeataXADriver registers a Seata XA driver wrapper for a vendor driver.
// Call this during application initialization before sql.Open uses driverName.
func RegisterSeataXADriver(driverName string, descriptor SeataDriverDescriptor) (err error) {
	if strings.TrimSpace(driverName) == "" {
		return errors.New("driver name is required")
	}
	desc, err := descriptor.internal()
	if err != nil {
		return err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("register seata xa driver %q: %v", driverName, recovered)
		}
	}()
	sql.Register(driverName, &seataXADriver{
		seataDriver: &seataDriver{
			branchType: branch.BranchTypeXA,
			transType:  types.XAMode,
			descriptor: desc,
			target:     desc.target,
			targetName: desc.targetName,
		},
	})
	return nil
}

var (
	mySQLDriverDescriptor = driverDescriptor{
		dbType:      types.DBTypeMySQL,
		target:      mysql.MySQLDriver{},
		targetName:  "mysql",
		parseDBName: parseMySQLDBName,
		newTableMetaCache: func(db *sql.DB, dbName string) datasource.TableMetaCache {
			return mysql2.NewTableMetaInstance(db, &mysql.Config{DBName: dbName})
		},
	}
	mariaDBDriverDescriptor = driverDescriptor{
		dbType:      types.DBTypeMARIADB,
		target:      mysql.MySQLDriver{},
		targetName:  "mysql",
		parseDBName: parseMySQLDBName,
		newTableMetaCache: func(db *sql.DB, dbName string) datasource.TableMetaCache {
			return mysql2.NewTableMetaInstance(db, &mysql.Config{DBName: dbName})
		},
	}
	postgresDriverDescriptor = driverDescriptor{
		dbType:      types.DBTypePostgreSQL,
		target:      stdlib.GetDefaultDriver(),
		targetName:  "pgx",
		parseDBName: parsePostgresDBName,
		newTableMetaCache: func(db *sql.DB, dbName string) datasource.TableMetaCache {
			return postgres2.NewTableMetaInstance(db, dbName)
		},
	}
)

func initDriver() {
	sql.Register(SeataATMySQLDriver, &seataATDriver{
		seataDriver: &seataDriver{
			branchType: branch.BranchTypeAT,
			transType:  types.ATMode,
			target:     mySQLDriverDescriptor.target,
			targetName: mySQLDriverDescriptor.targetName,
			descriptor: mySQLDriverDescriptor,
		},
	})

	sql.Register(SeataATPostgresDriver, &seataATDriver{
		seataDriver: &seataDriver{
			branchType: branch.BranchTypeAT,
			transType:  types.ATMode,
			descriptor: postgresDriverDescriptor,
			target:     postgresDriverDescriptor.target,
			targetName: postgresDriverDescriptor.targetName,
		},
	})

	sql.Register(SeataXAMySQLDriver, &seataXADriver{
		seataDriver: &seataDriver{
			branchType: branch.BranchTypeXA,
			transType:  types.XAMode,
			descriptor: mySQLDriverDescriptor,
			target:     mySQLDriverDescriptor.target,
			targetName: mySQLDriverDescriptor.targetName,
		},
	})

	sql.Register(SeataXAMariaDBDriver, &seataXADriver{
		seataDriver: &seataDriver{
			branchType: branch.BranchTypeXA,
			transType:  types.XAMode,
			descriptor: mariaDBDriverDescriptor,
			target:     mariaDBDriverDescriptor.target,
			targetName: mariaDBDriverDescriptor.targetName,
		},
	})

	sql.Register(SeataXAPostgresDriver, &seataXADriver{
		seataDriver: &seataDriver{
			branchType: branch.BranchTypeXA,
			transType:  types.XAMode,
			descriptor: postgresDriverDescriptor,
			target:     postgresDriverDescriptor.target,
			targetName: postgresDriverDescriptor.targetName,
		},
	})
}

type seataATDriver struct {
	*seataDriver
}

func (d *seataATDriver) OpenConnector(name string) (c driver.Connector, err error) {
	connector, err := d.seataDriver.OpenConnector(name)
	if err != nil {
		return nil, err
	}

	_connector, _ := connector.(*seataConnector)
	_connector.transType = types.ATMode

	return &seataATConnector{
		seataConnector: _connector,
	}, nil
}

type seataXADriver struct {
	*seataDriver
}

func (d *seataXADriver) OpenConnector(name string) (c driver.Connector, err error) {
	connector, err := d.seataDriver.OpenConnector(name)
	if err != nil {
		return nil, err
	}

	_connector, _ := connector.(*seataConnector)
	_connector.transType = types.XAMode

	return &seataXAConnector{
		seataConnector: _connector,
	}, nil
}

type seataDriver struct {
	branchType branch.BranchType
	transType  types.TransactionMode
	descriptor driverDescriptor
	target     driver.Driver
	targetName string
}

// Open never be called, because seataDriver implemented dri.DriverContext interface.
// reference package: datasource/sql [https://cs.opensource.google/go/go/+/master:src/database/sql/sql.go;l=813]
// and maybe the sql.BD will be call Driver() method, but it obtain the Driver is fron Connector that is proxed by seataConnector.
func (d *seataDriver) Open(name string) (driver.Conn, error) {
	return nil, errors.New(("operation unsupport."))
}

func (d *seataDriver) OpenConnector(name string) (c driver.Connector, err error) {
	c = &dsnConnector{dsn: name, driver: d.descriptor.target}
	if driverCtx, ok := d.descriptor.target.(driver.DriverContext); ok {
		c, err = driverCtx.OpenConnector(name)
		if err != nil {
			log.Errorf("open connector: %v", err)
			return nil, err
		}
	}

	dbType := d.descriptor.dbType
	if dbType == types.DBTypeUnknown {
		return nil, fmt.Errorf("unsupport conn type %d", dbType)
	}

	proxy, err := d.getOpenConnectorProxy(c, dbType, sql.OpenDB(c), name)
	if err != nil {
		log.Errorf("register resource: %v", err)
		return nil, err
	}

	return proxy, nil
}

func (d *seataDriver) getOpenConnectorProxy(connector driver.Connector, dbType types.DBType,
	db *sql.DB, dataSourceName string) (driver.Connector, error) {
	dbName, err := d.descriptor.parseDBName(dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("parse db name: %w", err)
	}
	options := []dbOption{
		withResourceID(parseResourceID(dataSourceName)),
		withTarget(db),
		withBranchType(d.branchType),
		withDBType(dbType),
		withDBName(dbName),
		withConnector(connector),
	}
	res, err := newResource(options...)
	if err != nil {
		log.Errorf("create new resource: %v", err)
		return nil, err
	}

	if dbType == types.DBTypeMySQL || dbType == types.DBTypeMARIADB {
		cfg, err := mysql.ParseDSN(dataSourceName)
		if err != nil {
			return nil, fmt.Errorf("parse mysql dsn: %w", err)
		}
		datasource.RegisterTableCache(dbType, mysql2.NewTableMetaInstance(db, cfg))
	}

	if d.descriptor.newTableMetaCache != nil {
		datasource.RegisterTableCache(dbType, d.descriptor.newTableMetaCache(db, dbName))
	}
	if err = datasource.GetDataSourceManager(d.branchType).RegisterResource(res); err != nil {
		log.Errorf("register resource: %v", err)
		return nil, err
	}
	return &seataConnector{
		transType:    d.transType,
		branchType:   d.branchType,
		res:          res,
		driver:       d,
		target:       connector,
		targetDriver: d.target,
		targetName:   d.targetName,
		dbType:       dbType,
		dbName:       dbName,
	}, nil
}

func parseMySQLDBName(dsn string) (string, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	return cfg.DBName, nil
}

func parsePostgresDBName(dsn string) (string, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return "", err
	}
	return cfg.Database, nil
}

func (d *seataDriver) getTargetDriverName() string {
	return d.targetName
}

type connectorMetadata struct {
	dbName string
}

func parseConnectorMetadata(dataSourceName string, dbType types.DBType) (*connectorMetadata, error) {
	switch dbType {
	case types.DBTypeMySQL, types.DBTypeMARIADB:
		cfg, err := mysql.ParseDSN(dataSourceName)
		if err != nil {
			return nil, fmt.Errorf("parse mysql dsn: %w", err)
		}
		return &connectorMetadata{dbName: cfg.DBName}, nil
	case types.DBTypePostgreSQL:
		cfg, err := pgx.ParseConfig(dataSourceName)
		if err != nil {
			return nil, fmt.Errorf("parse postgres dsn: %w", err)
		}
		return &connectorMetadata{dbName: cfg.Database}, nil
	default:
		return nil, fmt.Errorf("unsupported connector metadata for db type %s", dbType.String())
	}
}

type dsnConnector struct {
	dsn    string
	driver driver.Driver
}

func (t *dsnConnector) Connect(_ context.Context) (driver.Conn, error) {
	return t.driver.Open(t.dsn)
}

func (t *dsnConnector) Driver() driver.Driver {
	return t.driver
}

func parseResourceID(dsn string) string {
	i := strings.Index(dsn, "?")
	res := dsn
	if i > 0 {
		res = dsn[:i]
	}
	res = redactURLUserInfo(res)
	res = redactDSNUserInfo(res)
	res = redactKeyValueCredentials(res)
	return strings.ReplaceAll(res, ",", "|")
}

func redactURLUserInfo(resourceID string) string {
	u, err := url.Parse(resourceID)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User == nil {
		return resourceID
	}
	u.User = nil
	return u.String()
}

func redactDSNUserInfo(resourceID string) string {
	slash := strings.LastIndex(resourceID, "/")
	if slash < 0 {
		return resourceID
	}
	at := strings.LastIndex(resourceID[:slash], "@")
	if at < 0 {
		return resourceID
	}
	return resourceID[at+1:]
}

func redactKeyValueCredentials(resourceID string) string {
	if !strings.Contains(resourceID, "=") {
		return resourceID
	}
	fields := splitResourceIDFields(resourceID)
	if len(fields) == 0 {
		return resourceID
	}

	kept := make([]string, 0, len(fields))
	for _, field := range fields {
		key, _, ok := strings.Cut(field, "=")
		if !ok || !isCredentialResourceIDKey(strings.TrimSpace(key)) {
			kept = append(kept, field)
		}
	}
	if len(kept) == len(fields) {
		return resourceID
	}
	return strings.Join(kept, " ")
}

func splitResourceIDFields(resourceID string) []string {
	var fields []string
	start := -1
	inQuote := false
	escaped := false
	for i, r := range resourceID {
		if start < 0 {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ';' {
				continue
			}
			start = i
		}
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '\'' {
			inQuote = !inQuote
			continue
		}
		if !inQuote && (r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ';') {
			fields = append(fields, strings.TrimSpace(resourceID[start:i]))
			start = -1
		}
	}
	if start >= 0 {
		fields = append(fields, strings.TrimSpace(resourceID[start:]))
	}
	return fields
}

func isCredentialResourceIDKey(key string) bool {
	switch strings.ToLower(key) {
	case "user", "username", "password", "passwd":
		return true
	default:
		return false
	}
}

func selectDBVersion(ctx context.Context, conn driver.Conn) (string, error) {
	var rowsi driver.Rows
	var err error

	queryerCtx, ok := conn.(driver.QueryerContext)
	var queryer driver.Queryer
	if !ok {
		queryer, ok = conn.(driver.Queryer)
	}
	if ok {
		rowsi, err = util.CtxDriverQuery(ctx, queryerCtx, queryer, "SELECT VERSION()", nil)
		if err != nil {
			log.Errorf("ctx driver query: %+v", err)
			return "", err
		}
		defer func() {
			if rowsi != nil {
				rowsi.Close()
			}
		}()
	} else {
		log.Errorf("target conn should been driver.QueryerContext or driver.Queryer")
		return "", fmt.Errorf("invalid conn")
	}

	dest := make([]driver.Value, 1)
	var version string
	if err = rowsi.Next(dest); err != nil {
		if err == io.EOF {
			return version, nil
		}
		return "", err
	}
	if len(dest) != 1 {
		return "", errors.New("get db version is not column 1")
	}

	switch reflect.TypeOf(dest[0]).Kind() {
	case reflect.Slice, reflect.Array:
		val := reflect.ValueOf(dest[0]).Bytes()
		version = string(val)
	case reflect.String:
		version = reflect.ValueOf(dest[0]).String()
	default:
		return "", errors.New("get db version is not a string")
	}

	return version, nil
}
