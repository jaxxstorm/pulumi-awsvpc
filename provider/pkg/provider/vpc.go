// Copyright 2016-2021, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"fmt"

	"github.com/imdario/mergo"

	awsconfig "github.com/pulumi/pulumi-aws/sdk/v4/go/aws/config"
	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws/route53"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/jaxxstorm/pulumi-awsvpc/pkg/convert"
)

const vpcToken = "awsvpc:index:Vpc"

// The set of arguments for creating a StaticPage component resource.
type VPCArgs struct {
	// The CIDR for the created VPC
	AvailabilityZoneNames  []string              `pulumi:"availabilityZoneNames"`
	BaseCIDR               string                `pulumi:"baseCidr"`
	Tags                   pulumi.StringMapInput `pulumi:"tags"`
	ZoneName               pulumi.StringInput    `pulumi:"zoneName"`
	CreatePrivateZone      bool                  `pulumi:"createPrivateZone"`
	EnableS3Endpoint       bool                  `pulumi:"enableS3Endpoint"`
	EnableDynamoDBEndpoint bool                  `pulumi:"enableDynamoDBEndpoint"`
}

// The VPC component resource.
type VPC struct {
	pulumi.ResourceState

	VpcID            pulumi.StringOutput  `pulumi:"vpcId"`
	PublicSubnetIDs  pulumi.IDArrayOutput `pulumi:"publicSubnetIds"`
	PrivateSubnetIDs pulumi.IDArrayOutput `pulumi:"privateSubnetIds"`
}

func resourceTags(tags pulumi.StringMapInput, baseTags pulumi.StringMap) pulumi.StringMapInput {
	mergo.Merge(&tags, baseTags)
	return tags
}

// NewStaticPage creates a new StaticPage component resource.
func NewVPC(ctx *pulumi.Context,
	name string, args *VPCArgs, opts ...pulumi.ResourceOption) (*VPC, error) {
	if args == nil {
		args = &VPCArgs{}
	}

	component := &VPC{}
	err := ctx.RegisterComponentResource(vpcToken, name, component, opts...)
	if err != nil {
		return nil, err
	}

	region := awsconfig.GetRegion(ctx)

	// Create the VPC itself
	vpc, err := ec2.NewVpc(ctx, fmt.Sprintf("%s-vpc", name), &ec2.VpcArgs{
		CidrBlock:          pulumi.String(args.BaseCIDR),
		EnableDnsHostnames: pulumi.Bool(true),
		EnableDnsSupport:   pulumi.Bool(true),
		Tags: resourceTags(args.Tags, pulumi.StringMap{
			"Name": pulumi.Sprintf("%s-vpc", name),
		}),
	}, pulumi.Parent(component))
	if err != nil {
		return nil, fmt.Errorf("error creating vpc: %v", nil)
	}

	// Create an Internet gateway for internet access
	igw, err := ec2.NewInternetGateway(ctx, fmt.Sprintf("%s-igw", name), &ec2.InternetGatewayArgs{
		VpcId: vpc.ID(),
	}, pulumi.Parent(vpc))
	if err != nil {
		return nil, fmt.Errorf("error creating internet gateway: %v", nil)
	}

	// Optionally create a private zone
	if args.CreatePrivateZone {
		zone, err := route53.NewZone(ctx, fmt.Sprintf("%s-private-zone", name), &route53.ZoneArgs{
			Name: args.ZoneName,
			Vpcs: route53.ZoneVpcArray{
				&route53.ZoneVpcArgs{
					VpcId: vpc.ID(),
				},
			},
		}, pulumi.Parent(vpc))
		if err != nil {
			return nil, fmt.Errorf("error creating private Route53 zone: %v", nil)
		}

		// creates the DHCP option set
		dhcpOptionSet, err := ec2.NewVpcDhcpOptions(ctx, fmt.Sprintf("%s-dhcp-options", name), &ec2.VpcDhcpOptionsArgs{
			DomainName: zone.Name,
			DomainNameServers: pulumi.StringArray{
				pulumi.String("AmazonProvidedDNS"),
			},
			Tags: resourceTags(args.Tags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s DHCP Options", name),
			}),
		}, pulumi.Parent(vpc))
		if err != nil {
			return nil, err
		}
		_, err = ec2.NewVpcDhcpOptionsAssociation(ctx, fmt.Sprintf("%s-dhcp-options-assoc", name), &ec2.VpcDhcpOptionsAssociationArgs{
			VpcId:         vpc.ID(),
			DhcpOptionsId: dhcpOptionSet.ID(),
		}, pulumi.Parent(dhcpOptionSet))
		if err != nil {
			return nil, err
		}
	}

	// split the subnet CIDR into smaller subnets for each available zone
	privateSubnetCIDRs, publicSubnetCIDRs, err := SubnetDistributor(args.BaseCIDR, len(args.AvailabilityZoneNames))
	if err != nil {
		return nil, fmt.Errorf("unable to create valid subnets: %v", err) // FIXME: better error message
	}

	var privateSubnets []ec2.Subnet
	var privateSubnetIDs []pulumi.IDOutput

	for index, subnetCIDR := range privateSubnetCIDRs {
		subnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-private-%s", name, args.AvailabilityZoneNames[index]), &ec2.SubnetArgs{
			VpcId:            vpc.ID(),
			CidrBlock:        pulumi.String(subnetCIDR),
			AvailabilityZone: pulumi.String(args.AvailabilityZoneNames[index]),
			Tags: resourceTags(args.Tags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s-private-%s", name, args.AvailabilityZoneNames[index]),
			}),
		}, pulumi.Parent(vpc))

		privateSubnets = append(privateSubnets, *subnet)
		privateSubnetIDs = append(privateSubnetIDs, subnet.ID())

		if err != nil {
			return nil, fmt.Errorf("error creating subnet: %v", err)
		}
	}

	var publicSubnets []ec2.Subnet
	var publicSubnetIDs []pulumi.IDOutput

	for index, subnetCIDR := range publicSubnetCIDRs {
		subnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-public-%s", name, args.AvailabilityZoneNames[index]), &ec2.SubnetArgs{
			VpcId:               vpc.ID(),
			CidrBlock:           pulumi.String(subnetCIDR),
			MapPublicIpOnLaunch: pulumi.Bool(true),
			AvailabilityZone:    pulumi.String(args.AvailabilityZoneNames[index]),
			Tags: resourceTags(args.Tags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s-public-%s", name, args.AvailabilityZoneNames[index]),
			}),
		}, pulumi.Parent(vpc))

		publicSubnets = append(publicSubnets, *subnet)
		publicSubnetIDs = append(publicSubnetIDs, subnet.ID())

		if err != nil {
			return nil, fmt.Errorf("error creating subnet: %v", err)
		}
	}

	// adopt the default route table and make it usable for public subnets
	publicRouteTable, err := ec2.NewDefaultRouteTable(ctx, fmt.Sprintf("%s-public-rt", name), &ec2.DefaultRouteTableArgs{
		DefaultRouteTableId: vpc.DefaultRouteTableId,
		Tags: resourceTags(args.Tags, pulumi.StringMap{
			"Name": pulumi.Sprintf("%s Public Route Table", name),
		}),
	}, pulumi.Parent(vpc))
	if err != nil {
		return nil, fmt.Errorf("error creating public route table: %v", err)
	}

	// route all public subnets to internet gateway
	_, err = ec2.NewRoute(ctx, fmt.Sprintf("%s-route-public-sn-to-igw", name), &ec2.RouteArgs{
		RouteTableId:         publicRouteTable.ID(),
		DestinationCidrBlock: pulumi.String("0.0.0.0/0"),
		GatewayId:            igw.ID(),
	}, pulumi.Parent(publicRouteTable))
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("error public route to igw: %v", err)
	}

	for index, subnet := range publicSubnets {
		_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("%s-public-rta-%s", name, args.AvailabilityZoneNames[index]), &ec2.RouteTableAssociationArgs{
			SubnetId:     subnet.ID(),
			RouteTableId: publicRouteTable.ID(),
		}, pulumi.Parent(publicRouteTable))
		if err != nil {
			return nil, err
		}
	}

	// sets up the routing for private subnets via a NAT gateway
	for index, subnet := range privateSubnets {
		elasticIP, err := ec2.NewEip(ctx, fmt.Sprintf("%s-nat-%s", name, args.AvailabilityZoneNames[index]), &ec2.EipArgs{
			Tags: resourceTags(args.Tags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s NAT Gateway EIP %s", name, args.AvailabilityZoneNames[index]),
			}),
		}, pulumi.Parent(&subnet))
		if err != nil {
			return nil, err
		}

		natGateway, err := ec2.NewNatGateway(ctx, fmt.Sprintf("%s-nat-gateway-%s", name, args.AvailabilityZoneNames[index]), &ec2.NatGatewayArgs{
			AllocationId: elasticIP.ID(),
			SubnetId:     publicSubnets[index].ID(),
			Tags: resourceTags(args.Tags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s NAT Gateway %s", name, args.AvailabilityZoneNames[index]),
			}),
		}, pulumi.Parent(&subnet))
		if err != nil {
			return nil, err
		}

		privateRouteTable, err := ec2.NewRouteTable(ctx, fmt.Sprintf("%s-private-rt-%s", name, args.AvailabilityZoneNames[index]), &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
			Tags: resourceTags(args.Tags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s Private Subnet RT %s", name, args.AvailabilityZoneNames[index]),
			}),
		}, pulumi.Parent(vpc))
		if err != nil {
			return nil, err
		}

		_, err = ec2.NewRoute(ctx, fmt.Sprintf("%s-route-private-sn-to-nat-%s", name, args.AvailabilityZoneNames[index]), &ec2.RouteArgs{
			RouteTableId:         privateRouteTable.ID(),
			DestinationCidrBlock: pulumi.String("0.0.0.0/0"),
			NatGatewayId:         natGateway.ID(),
		}, pulumi.Parent(privateRouteTable))
		if err != nil {
			return nil, err
		}

		_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("%s-private-rta-%s", name, args.AvailabilityZoneNames[index]), &ec2.RouteTableAssociationArgs{
			SubnetId:     subnet.ID(),
			RouteTableId: privateRouteTable.ID(),
		}, pulumi.Parent(privateRouteTable))
		if err != nil {
			return nil, err
		}
	}

	// set up endpoints
	if args.EnableS3Endpoint {
		_, err = ec2.NewVpcEndpoint(ctx, fmt.Sprintf("%s-s3-endpoint", name), &ec2.VpcEndpointArgs{
			VpcId:       vpc.ID(),
			ServiceName: pulumi.String(fmt.Sprintf("com.amazonaws.%s.s3", region)),
		}, pulumi.Parent(vpc))
		if err != nil {
			return nil, err
		}
	}
	if args.EnableDynamoDBEndpoint {
		_, err = ec2.NewVpcEndpoint(ctx, fmt.Sprintf("%s-dynamodb-endpoint", name), &ec2.VpcEndpointArgs{
			VpcId:       vpc.ID(),
			ServiceName: pulumi.String(fmt.Sprintf("com.amazonaws.%s.dynamodb", region)),
		}, pulumi.Parent(vpc))
		if err != nil {
			return nil, err
		}
	}

	component.VpcID = vpc.ID().ToStringOutput()
	component.PublicSubnetIDs = convert.IDOutputSlicetoIDArrayOutput(publicSubnetIDs)
	component.PrivateSubnetIDs = convert.IDOutputSlicetoIDArrayOutput(privateSubnetIDs)

	outputs := pulumi.Map{
		"vpcId":            component.VpcID,
		"publicSubnetIds":  component.PublicSubnetIDs,
		"privateSubnetIds": component.PrivateSubnetIDs,
	}

	if err := ctx.RegisterResourceOutputs(component, outputs); err != nil {
		return nil, err
	}

	return component, nil
}
