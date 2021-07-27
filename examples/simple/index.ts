import * as awsvpc from "@jaxxstorm/pulumi-awsvpc";


const vpc = new awsvpc.Vpc("example", {
    baseCidr: "172.0.0.0/24",
    availabilityZoneNames: [
        "us-west-2a",
        "us-west-2b",
        "us-west-2c"
    ]
})

export const vpcId = vpc.vpcId
