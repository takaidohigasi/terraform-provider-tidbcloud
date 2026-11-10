package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	mockClient "github.com/tidbcloud/terraform-provider-tidbcloud/mock"
	"github.com/tidbcloud/terraform-provider-tidbcloud/tidbcloud"
	clusterV1beta1 "github.com/tidbcloud/tidbcloud-cli/pkg/tidbcloud/v1beta1/serverless/cluster"
)

func TestAccServerlessClusterResource(t *testing.T) {
	serverlessClusterResourceName := "tidbcloud_serverless_cluster.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccServerlessClusterResourceConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(serverlessClusterResourceName, "name", "test-tf"),
					resource.TestCheckResourceAttr(serverlessClusterResourceName, "region.region_id", "us-east-1"),
					resource.TestCheckResourceAttr(serverlessClusterResourceName, "endpoints.public.port", "4000"),
					resource.TestCheckResourceAttr(serverlessClusterResourceName, "endpoints.private.port", "4000"),
				),
			},
			// Update testing
			{
				Config: testAccServerlessClusterResourceUpdateConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("tidbcloud_serverless_cluster.test", "name", "test-tf2"),
				),
			},
		},
	})
}

func TestUTServerlessClusterResource(t *testing.T) {
	setupTestEnv()

	ctrl := gomock.NewController(t)
	s := mockClient.NewMockTiDBCloudServerlessClient(ctrl)
	defer HookGlobal(&NewServerlessClient, func(publicKey string, privateKey string, serverlessEndpoint string, userAgent string) (tidbcloud.TiDBCloudServerlessClient, error) {
		return s, nil
	})()

	clusterId := "cluster_id"
	regionName := "regions/aws-us-east-1"
	displayName := "test-tf"

	createClusterResp := clusterV1beta1.TidbCloudOpenApiserverlessv1beta1Cluster{}
	createClusterResp.UnmarshalJSON([]byte(testUTTidbCloudOpenApiserverlessv1beta1Cluster(clusterId, regionName, displayName, string(clusterV1beta1.COMMONV1BETA1CLUSTERSTATE_CREATING))))
	getClusterResp := clusterV1beta1.TidbCloudOpenApiserverlessv1beta1Cluster{}
	getClusterResp.UnmarshalJSON([]byte(testUTTidbCloudOpenApiserverlessv1beta1Cluster(clusterId, regionName, displayName, string(clusterV1beta1.COMMONV1BETA1CLUSTERSTATE_ACTIVE))))
	getClusterAfterUpdateResp := clusterV1beta1.TidbCloudOpenApiserverlessv1beta1Cluster{}
	getClusterAfterUpdateResp.UnmarshalJSON([]byte(testUTTidbCloudOpenApiserverlessv1beta1Cluster(clusterId, regionName, "test-tf2", string(clusterV1beta1.COMMONV1BETA1CLUSTERSTATE_ACTIVE))))
	updateClusterSuccessResp := clusterV1beta1.TidbCloudOpenApiserverlessv1beta1Cluster{}
	updateClusterSuccessResp.UnmarshalJSON([]byte(testUTTidbCloudOpenApiserverlessv1beta1Cluster(clusterId, regionName, "test-tf2", string(clusterV1beta1.COMMONV1BETA1CLUSTERSTATE_ACTIVE))))

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&createClusterResp, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterId, clusterV1beta1.CLUSTERSERVICEGETCLUSTERVIEWPARAMETER_BASIC).Return(&getClusterResp, nil).AnyTimes()
	gomock.InOrder(
		s.EXPECT().GetCluster(gomock.Any(), clusterId, clusterV1beta1.CLUSTERSERVICEGETCLUSTERVIEWPARAMETER_FULL).Return(&getClusterResp, nil).Times(3),
		s.EXPECT().GetCluster(gomock.Any(), clusterId, clusterV1beta1.CLUSTERSERVICEGETCLUSTERVIEWPARAMETER_FULL).Return(&getClusterAfterUpdateResp, nil).Times(2),
	)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterId).Return(&getClusterResp, nil)
	s.EXPECT().PartialUpdateCluster(gomock.Any(), clusterId, gomock.Any()).Return(&updateClusterSuccessResp, nil)

	testServerlessClusterResource(t)
}

func testServerlessClusterResource(t *testing.T) {
	serverlessClusterResourceName := "tidbcloud_serverless_cluster.test"
	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read serverless cluster resource
			{
				ExpectNonEmptyPlan: true,
				Config:             testUTServerlessClusterResourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(serverlessClusterResourceName, "cluster_id"),
					resource.TestCheckResourceAttr(serverlessClusterResourceName, "display_name", "test-tf"),
					resource.TestCheckResourceAttr(serverlessClusterResourceName, "region.name", "regions/aws-us-east-1"),
					resource.TestCheckResourceAttr(serverlessClusterResourceName, "spending_limit.monthly", "0"),
					resource.TestCheckNoResourceAttr(serverlessClusterResourceName, "auto_scaling.min_rcu"),
					resource.TestCheckNoResourceAttr(serverlessClusterResourceName, "auto_scaling.max_rcu"),
				),
			},
			// // Update correctly
			{
				ExpectNonEmptyPlan: true,
				Config:             testUTServerlessClusterResourceUpdateConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(serverlessClusterResourceName, "display_name", "test-tf2"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestRefreshServerlessClusterResourceDataNormalizesZeroAutoScaling(t *testing.T) {
	clusterId := "cluster_id"
	regionName := "regions/aws-us-east-1"
	displayName := "test-tf"

	getClusterResp := clusterV1beta1.TidbCloudOpenApiserverlessv1beta1Cluster{}
	err := getClusterResp.UnmarshalJSON([]byte(testUTTidbCloudOpenApiserverlessv1beta1Cluster(clusterId, regionName, displayName, string(clusterV1beta1.COMMONV1BETA1CLUSTERSTATE_ACTIVE))))
	if err != nil {
		t.Fatalf("failed to unmarshal cluster fixture: %v", err)
	}

	var data serverlessClusterResourceData
	err = refreshServerlessClusterResourceData(context.Background(), &getClusterResp, &data)
	if err != nil {
		t.Fatalf("failed to refresh serverless cluster data: %v", err)
	}
	if !data.AutoScaling.IsNull() {
		t.Fatalf("expected zero auto scaling response to be normalized to null, got %#v", data.AutoScaling)
	}
}

func TestRefreshServerlessClusterResourceDataPreservesExplicitZeroAutoScaling(t *testing.T) {
	clusterId := "cluster_id"
	regionName := "regions/aws-us-east-1"
	displayName := "test-tf"

	getClusterResp := clusterV1beta1.TidbCloudOpenApiserverlessv1beta1Cluster{}
	err := getClusterResp.UnmarshalJSON([]byte(testUTTidbCloudOpenApiserverlessv1beta1Cluster(clusterId, regionName, displayName, string(clusterV1beta1.COMMONV1BETA1CLUSTERSTATE_ACTIVE))))
	if err != nil {
		t.Fatalf("failed to unmarshal cluster fixture: %v", err)
	}

	ctx := context.Background()
	data := serverlessClusterResourceData{
		AutoScaling: mustAutoScalingObject(t, ctx, 0, 0),
	}

	err = refreshServerlessClusterResourceData(ctx, &getClusterResp, &data)
	if err != nil {
		t.Fatalf("failed to refresh serverless cluster data: %v", err)
	}
	if data.AutoScaling.IsNull() {
		t.Fatal("expected explicit zero auto scaling to be preserved")
	}

	var refreshed autoScaling
	diags := data.AutoScaling.As(ctx, &refreshed, basetypes.ObjectAsOptions{})
	if diags.HasError() {
		t.Fatalf("failed to decode refreshed auto scaling: %v", diags)
	}
	if refreshed.MinRCU.ValueInt64() != 0 || refreshed.MaxRCU.ValueInt64() != 0 {
		t.Fatalf("expected zero auto scaling, got min=%d max=%d", refreshed.MinRCU.ValueInt64(), refreshed.MaxRCU.ValueInt64())
	}
}

func TestRefreshServerlessClusterResourceDataPreservesPlannedZeroAutoScaling(t *testing.T) {
	clusterId := "cluster_id"
	regionName := "regions/aws-us-east-1"
	displayName := "test-tf"

	getClusterResp := clusterV1beta1.TidbCloudOpenApiserverlessv1beta1Cluster{}
	err := getClusterResp.UnmarshalJSON([]byte(testUTTidbCloudOpenApiserverlessv1beta1Cluster(clusterId, regionName, displayName, string(clusterV1beta1.COMMONV1BETA1CLUSTERSTATE_ACTIVE))))
	if err != nil {
		t.Fatalf("failed to unmarshal cluster fixture: %v", err)
	}

	ctx := context.Background()
	plan := serverlessClusterResourceData{
		AutoScaling: mustAutoScalingObject(t, ctx, 0, 0),
	}

	err = refreshServerlessClusterResourceData(ctx, &getClusterResp, &plan)
	if err != nil {
		t.Fatalf("failed to refresh serverless cluster data: %v", err)
	}
	if plan.AutoScaling.IsNull() {
		t.Fatal("expected planned zero auto scaling to be preserved")
	}
}

func mustAutoScalingObject(t *testing.T, ctx context.Context, minRCU, maxRCU int64) types.Object {
	t.Helper()

	as := autoScaling{
		MinRCU: types.Int64Value(minRCU),
		MaxRCU: types.Int64Value(maxRCU),
	}
	value, diags := types.ObjectValueFrom(ctx, autoScalingAttrTypes, as)
	if diags.HasError() {
		t.Fatalf("failed to build auto scaling object: %v", diags)
	}
	return value
}

func testAccServerlessClusterResourceConfig() string {
	return `
resource "tidbcloud_serverless_cluster" "test" {
   display_name = "test-tf"
   region = {
      name = "regions/aws-us-east-1"
   }
}
`
}

func testAccServerlessClusterResourceUpdateConfig() string {
	return `
resource "tidbcloud_serverless_cluster" "test" {
   display_name = "test-tf2"
   region = {
      name = "regions/aws-us-east-1"
   }
}
`
}

func testUTServerlessClusterResourceConfig() string {
	return `
resource "tidbcloud_serverless_cluster" "test" {
   display_name = "test-tf"
   region = {
      name = "regions/aws-us-east-1"
   }
}
`
}

func testUTServerlessClusterResourceUpdateConfig() string {
	return `
resource "tidbcloud_serverless_cluster" "test" {
   display_name = "test-tf2"
   region = {
      name = "regions/aws-us-east-1"
   }
}
`
}

func testUTTidbCloudOpenApiserverlessv1beta1Cluster(clusterId, regionName, displayName, state string) string {
	return fmt.Sprintf(`{
	"name": "clusters/%s",
    "clusterId": "%s",
    "displayName": "%s",
	"region": {
        "name": "%s",
        "regionId": "us-east-1",
        "cloudProvider": "aws",
        "displayName": "N. Virginia (us-east-1)",
        "provider": "aws"
    },
    "spendingLimit": {
        "monthly": 0
    },
    "autoScaling": {
        "minRcu": 0,
        "maxRcu": 0
    },
    "automatedBackupPolicy": {
        "startTime": "07:00",
        "retentionDays": 1
    },
    "endpoints": {
        "public": {
            "host": "gateway01.us-east-1.dev.shared.aws.tidbcloud.com",
            "port": 4000,
            "disabled": false,
            "authorizedNetworks": [
                {
                    "startIpAddress": "0.0.0.0",
                    "endIpAddress": "255.255.255.255",
                    "displayName": "Allow_all_public_connections"
                }
            ]
        },
        "private": {
            "host": "gateway01-privatelink.us-east-1.dev.shared.aws.tidbcloud.com",
            "port": 4000,
            "aws": {
                "serviceName": "com.amazonaws.vpce.us-east-1.vpce-svc-03342995daxxxxxxx",
                "availabilityZone": [
                    "use1-az1"
                ]
            }
        }
    },
    "rootPassword": "",
    "encryptionConfig": {
        "enhancedEncryptionEnabled": false
    },
    "highAvailabilityType": "ZONAL",
    "version": "v7.5.2",
    "createdBy": "apikey-xxxxxxx",
    "userPrefix": "2vphu1xxxxxxxx",
    "state": "%s",
    "usage": {
        "requestUnit": "0",
        "rowBasedStorage": 1.1222448348999023,
        "columnarStorage": 0
    },
    "labels": {
        "tidb.cloud/organization": "xxxxx",
        "tidb.cloud/project": "xxxxxxx"
    },
    "annotations": {
        "tidb.cloud/available-features": "DISABLE_PUBLIC_LB,DELEGATE_USER",
        "tidb.cloud/has-set-password": "false"
    },
    "createTime": "2025-02-26T07:09:31.869Z",
    "updateTime": "2025-02-26T07:09:51Z",
    "auditLogConfig": {
        "enabled": false,
        "unredacted": false
	}
}`, clusterId, clusterId, displayName, regionName, state)
}
