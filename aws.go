package main

import (
	"context"
	"log"
	"path"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type Server struct {
	Name, Address, Profile, Key, Platform string
}

func TagWithName(tags []types.Tag, name string) string {
	for _, tag := range tags {
		if *tag.Key == "Name" {
			return *tag.Value
		}
	}
	return ""
}

func Instances(profile, keysBasePath string) []Server {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithSharedConfigProfile(profile))
	if err != nil {
		log.Fatal(err)
	}
	client := ec2.NewFromConfig(cfg)
	name := "instance-state-name"
	output, err := client.DescribeInstances(context.TODO(), &ec2.DescribeInstancesInput{
		Filters: []types.Filter{{
			Name:   &name,
			Values: []string{"running"},
		}},
	})
	if err != nil {
		log.Fatal(err)
	}
	var res []Server
	for _, r := range output.Reservations {
		for _, i := range r.Instances {
			s := Server{TagWithName(i.Tags, "Name"), *i.PublicIpAddress, profile, path.Join(keysBasePath, *i.KeyName), string(i.Platform)}
			res = append(res, s)
		}
	}
	return res
}
