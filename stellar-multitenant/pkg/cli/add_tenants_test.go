package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	cmdDB "github.com/stellar/stellar-disbursement-platform-backend/cmd/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

// AssertEntriesContains asserts that the entries []logrus.Entry slice contain the provided message.
var AssertEntriesContains = func(t *testing.T, entries []logrus.Entry, message string) {
	t.Helper()

	entriesContain := slices.ContainsFunc(entries, func(e logrus.Entry) bool {
		return e.Message == message
	})

	assert.True(t, entriesContain, fmt.Sprintf("entries should contain message: %s", message))
}

func Test_validateTenantNameArg(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{
			name: "orgname",
			err:  nil,
		},
		{
			name: "orgname-ukraine",
			err:  nil,
		},
		{
			name: "ORGNAME",
			err:  errors.New(`invalid tenant name "ORGNAME". It should only contains lower case letters and dash (-)`),
		},
		{
			name: "orgname org",
			err:  errors.New(`invalid tenant name "orgname org". It should only contains lower case letters and dash (-)`),
		},
		{
			name: "orgname126",
			err:  errors.New(`invalid tenant name "orgname126". It should only contains lower case letters and dash (-)`),
		},
		{
			name: "@rgn#ame$",
			err:  errors.New(`invalid tenant name "@rgn#ame$". It should only contains lower case letters and dash (-)`),
		},
		{
			name: "orgname_ukraine",
			err:  errors.New(`invalid tenant name "orgname_ukraine". It should only contains lower case letters and dash (-)`),
		},
	}

	for _, tc := range testCases {
		err := validateTenantNameArg(&cobra.Command{}, []string{tc.name})
		if tc.err != nil {
			assert.Equal(t, tc.err, err)
		} else {
			assert.Nil(t, err)
		}
	}
}

func Test_executeAddTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	messengerClientMock := &message.MessengerClientMock{}
	messengerClientMock.
		On("SendMessage", mock.AnythingOfType("message.Message")).
		Return(nil)

	ctx := context.Background()

	tenantName := "myorg"
	userFirstName := "First"
	userLastName := "Last"
	userEmail := "email@email.com"
	organizationName := "My Org"
	uiBaseURL := "http://localhost:3000"
	networkType := "testnet"
	encryptionPassphrase := keypair.MustRandom().Seed()
	distributionAcc := keypair.MustRandom()
	distributionAccPrivKey := distributionAcc.Seed()
	distributionAccPubKey := distributionAcc.Address()

	distAccResolverOpts := signing.DistributionAccountResolverOptions{
		AdminDBConnectionPool:            dbConnectionPool,
		HostDistributionAccountPublicKey: distributionAccPubKey,
	}
	distAccResolver, err := signing.NewDistributionAccountResolver(distAccResolverOpts)
	require.NoError(t, err)

	txSubOpts := di.TxSubmitterEngineOptions{
		SignatureServiceOptions: signing.SignatureServiceOptions{
			DistributionSignerType:      signing.DistributionAccountEnvSignatureClientType,
			DistAccEncryptionPassphrase: encryptionPassphrase,
			ChAccEncryptionPassphrase:   encryptionPassphrase,
			DistributionPrivateKey:      distributionAccPrivKey,
			NetworkPassphrase:           network.TestNetworkPassphrase,
			DistributionAccountResolver: distAccResolver,
			DBConnectionPool:            dbConnectionPool,
		},
		HorizonURL: horizonclient.DefaultTestNetClient.HorizonURL,
		MaxBaseFee: 100,
	}

	tenantsOpts := AddTenantsCommandOptions{
		SDPUIBaseURL:                            &uiBaseURL,
		NetworkType:                             networkType,
		TenantAccountNativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
	}

	t.Run("adds a new tenant successfully", func(t *testing.T) {
		di.ClearInstancesTestHelper(t)
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		err := executeAddTenant(ctx, dbConnectionPool, dbConnectionPool, tenantName, userFirstName, userLastName, userEmail, organizationName, messengerClientMock, tenantsOpts, txSubOpts, distAccResolverOpts)
		assert.Nil(t, err)

		const q = "SELECT id FROM tenants WHERE name = $1"
		var tenantID string
		err = dbConnectionPool.GetContext(ctx, &tenantID, q, tenantName)
		require.NoError(t, err)

		entries := getEntries()
		AssertEntriesContains(t, entries, "tenant myorg added successfully")
		AssertEntriesContains(t, entries, fmt.Sprintf("tenant ID: %s", tenantID))
	})

	t.Run("duplicated tenant name", func(t *testing.T) {
		di.ClearInstancesTestHelper(t)
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		err := executeAddTenant(ctx, dbConnectionPool, dbConnectionPool, tenantName, userFirstName, userLastName, userEmail, organizationName, messengerClientMock, tenantsOpts, txSubOpts, distAccResolverOpts)
		assert.Nil(t, err)

		err = executeAddTenant(ctx, dbConnectionPool, dbConnectionPool, tenantName, userFirstName, userLastName, userEmail, organizationName, messengerClientMock, tenantsOpts, txSubOpts, distAccResolverOpts)
		assert.ErrorIs(t, err, tenant.ErrDuplicatedTenantName)

		const q = "SELECT id FROM tenants WHERE name = $1"
		var tenantID string
		err = dbConnectionPool.GetContext(ctx, &tenantID, q, tenantName)
		require.NoError(t, err)

		entries := getEntries()
		AssertEntriesContains(t, entries, "tenant myorg added successfully")
		AssertEntriesContains(t, entries, fmt.Sprintf("tenant ID: %s", tenantID))
	})

	messengerClientMock.AssertExpectations(t)
}

func Test_AddTenantsCmd(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// Run TSS migrations in tss schema:
	manager, err := cmdDB.NewTSSDatabaseMigrationManager(dbConnectionPool)
	require.NoError(t, err)
	err = manager.CreateTSSSchemaIfNeeded(ctx)
	require.NoError(t, err)
	tssDNS, err := router.GetDNSForTSS(dbt.DSN)
	require.NoError(t, err)
	err = cmdDB.RunTSSMigrations(ctx, tssDNS, migrate.Up, 0)
	require.NoError(t, err)

	t.Setenv("DISTRIBUTION_SIGNER_TYPE", "DISTRIBUTION_ACCOUNT_ENV")
	encryptionPassphrase := keypair.MustRandom().Seed()
	t.Setenv("CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE", encryptionPassphrase)
	t.Setenv("DISTRIBUTION_ACCOUNT_ENCRYPTION_PASSPHRASE", encryptionPassphrase)
	t.Setenv("DISTRIBUTION_PUBLIC_KEY", "GDAZUHI4ARV73G3FI4JEZP57MPQTJ5I6BW7VZLNVHQJPANKPUGY2SDUY")
	t.Setenv("DISTRIBUTION_SEED", "SBIIOER5NAQTMFIPCRDDSQSCIMVPMIEPZEIZSBIGYPDCU6I5LLRSODK7")

	t.Run("shows usage", func(t *testing.T) {
		di.ClearInstancesTestHelper(t)

		out := new(strings.Builder)
		mockCmd := cobra.Command{}
		mockCmd.AddCommand(AddTenantsCmd())
		mockCmd.SetOut(out)
		mockCmd.SetErr(out)
		mockCmd.SetArgs([]string{"add-tenants"})
		err := mockCmd.ExecuteContext(ctx)
		assert.EqualError(t, err, "accepts 5 arg(s), received 0")

		expectErrMsg := "Error: accepts 5 arg(s), received 0"
		expectUsageMessages := []string{
			"Usage:",
			"add-tenants [flags]",
			"Examples:",
			"add-tenants [tenant name] [user first name] [user last name] [user email] [organization name]",
			"Flags:",
			"--aws-access-key-id string",
			"--aws-region string",
			"--aws-secret-access-key string",
			"--channel-account-encryption-passphrase string",
			"--distribution-account-encryption-passphrase string",
			"--distribution-public-key string",
			"--distribution-seed string",
			"--distribution-signer-type string",
			"--email-sender-type string",
			"-h, --help",
			"--horizon-url string",
			"--max-base-fee int",
			"--network-passphrase string",
			"--network-type string",
			"--sdp-ui-base-url string",
			"--tenant-xlm-bootstrap-amount int",
		}
		outputStr := out.String()
		assert.Contains(t, outputStr, expectErrMsg)
		for _, expected := range expectUsageMessages {
			assert.Contains(t, outputStr, expected)
		}

		out.Reset()
		mockCmd.SetArgs([]string{"add-tenants", "--help"})
		err = mockCmd.ExecuteContext(ctx)
		require.NoError(t, err)
		outputStr = out.String()
		for _, expected := range expectUsageMessages {
			assert.Contains(t, outputStr, expected)
		}
	})

	t.Run("adds new tenant successfully testnet", func(t *testing.T) {
		di.ClearInstancesTestHelper(t)

		tenantName := "unhcr"
		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		organizationName := "UNHCR"

		out := new(strings.Builder)
		rootCmd := rootCmd()
		rootCmd.AddCommand(AddTenantsCmd())
		rootCmd.SetOut(out)
		rootCmd.SetErr(out)
		rootCmd.SetArgs([]string{
			"add-tenants", tenantName, userFirstName, userLastName, userEmail, organizationName,
			"--email-sender-type", "DRY_RUN",
			"--network-type", "testnet",
			"--multitenant-db-url", dbt.DSN,
		})
		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		err := rootCmd.ExecuteContext(ctx)
		require.NoError(t, err)
		assert.Empty(t, out.String())

		const q = "SELECT id FROM tenants WHERE name = $1"
		var tenantID string
		err = dbConnectionPool.GetContext(ctx, &tenantID, q, tenantName)
		require.NoError(t, err)

		entries := getEntries()
		AssertEntriesContains(t, entries, "tenant unhcr added successfully")
		AssertEntriesContains(t, entries, fmt.Sprintf("tenant ID: %s", tenantID))

		// Connecting to the new schema
		schemaName := fmt.Sprintf("sdp_%s", tenantName)
		dataSourceName, err := dbConnectionPool.DSN(ctx)
		require.NoError(t, err)
		u, err := url.Parse(dataSourceName)
		require.NoError(t, err)
		uq := u.Query()
		uq.Set("search_path", schemaName)
		u.RawQuery = uq.Encode()

		tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
		require.NoError(t, err)
		defer tenantSchemaConnectionPool.Close()

		expectedTablesAfterMigrationsApplied := []string{
			"assets",
			"auth_migrations",
			"auth_user_mfa_codes",
			"auth_user_password_reset",
			"auth_users",
			"countries",
			"disbursements",
			"sdp_migrations",
			"messages",
			"organizations",
			"payments",
			"receiver_verifications",
			"receiver_wallets",
			"receivers",
			"wallets",
			"wallets_assets",
		}
		tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName, expectedTablesAfterMigrationsApplied)
		tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "XLM:"})
		tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Demo Wallet", "Vibrant Assist"})
		tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
	})

	t.Run("adds new tenant successfully pubnet", func(t *testing.T) {
		di.ClearInstancesTestHelper(t)

		tenantName := "irc"
		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		organizationName := "UNHCR"

		out := new(strings.Builder)
		rootCmd := rootCmd()
		rootCmd.AddCommand(AddTenantsCmd())
		rootCmd.SetOut(out)
		rootCmd.SetErr(out)
		rootCmd.SetArgs([]string{"add-tenants", tenantName, userFirstName, userLastName, userEmail, organizationName, "--email-sender-type", "DRY_RUN", "--network-type", "pubnet", "--multitenant-db-url", dbt.DSN})
		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		err := rootCmd.ExecuteContext(ctx)
		require.NoError(t, err)
		assert.Empty(t, out.String())

		const q = "SELECT id FROM tenants WHERE name = $1"
		var tenantID string
		err = dbConnectionPool.GetContext(ctx, &tenantID, q, tenantName)
		require.NoError(t, err)

		entries := getEntries()
		AssertEntriesContains(t, entries, "tenant irc added successfully")
		AssertEntriesContains(t, entries, fmt.Sprintf("tenant ID: %s", tenantID))

		// Connecting to the new schema
		schemaName := fmt.Sprintf("sdp_%s", tenantName)
		dataSourceName, err := dbConnectionPool.DSN(ctx)
		require.NoError(t, err)
		u, err := url.Parse(dataSourceName)
		require.NoError(t, err)
		uq := u.Query()
		uq.Set("search_path", schemaName)
		u.RawQuery = uq.Encode()

		tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
		require.NoError(t, err)
		defer tenantSchemaConnectionPool.Close()

		expectedTablesAfterMigrationsApplied := []string{
			"assets",
			"auth_migrations",
			"auth_user_mfa_codes",
			"auth_user_password_reset",
			"auth_users",
			"countries",
			"disbursements",
			"sdp_migrations",
			"messages",
			"organizations",
			"payments",
			"receiver_verifications",
			"receiver_wallets",
			"receivers",
			"wallets",
			"wallets_assets",
		}
		tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName, expectedTablesAfterMigrationsApplied)
		tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", "XLM:"})
		tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Freedom Wallet", "Vibrant Assist RC", "Vibrant Assist"})
		tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
	})
}
