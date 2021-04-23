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

	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws/route53"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// The set of arguments for creating a StaticPage component resource.
type VPCArgs struct {
	// The CIDR for the created VPC
	AvailabilityZoneNames pulumi.StringArrayInput `pulumi:"availabilityZoneNames"`
	BaseCIDR              pulumi.String           `pulumi:"baseCidr"`
	Tags                  pulumi.StringMapInput   `pulumi:"tags"`
	ZoneName              pulumi.StringInput      `pulumi:"zoneName"`
	CreatePrivateZone     bool                    `pulumi:"createPrivateZone"`
}

// The VPC component resource.
type VPC struct {
	pulumi.ResourceState

	VpcID pulumi.StringOutput `pulumi:"vpcId"`
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
	err := ctx.RegisterComponentResource("awsvpc:index:Vpc", name, component, opts...)
	if err != nil {
		return nil, err
	}

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

	_, err = ec2.NewInternetGateway(ctx, fmt.Sprintf("%s-igw", name), &ec2.InternetGatewayArgs{
		VpcId: vpc.ID(),
	}, pulumi.Parent(vpc))
	if err != nil {
		return nil, fmt.Errorf("error creating internet gateway: %v", nil)
	}

	if args.CreatePrivateZone {
		_, err = route53.NewZone(ctx, fmt.Sprintf("%s-private-zone", name), &route53.ZoneArgs{
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
	}

	privateSubnets, publicSubnets, err := SubnetDistributor(args.BaseCIDR, len(args.AvailabilityZoneNames))

	component.VpcID = vpc.ID().ToStringOutput()

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"vpcId": vpc.ID(),
	}); err != nil {
		return nil, err
	}

	return component, nil
}
