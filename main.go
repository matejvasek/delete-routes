package main

import (
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"log"
	"os"
	"regexp"
	"strings"
)

func main() {
	session := session.Must(session.NewSession())
	client := route53.New(session)

	regex := ""
	zoneName := ""
	flag.StringVar(&regex, "name-regex", "", "regex to search record by")
	flag.StringVar(&zoneName, "zone", "", "zone where the records are")
	flag.Parse()

	if regex == "" || zoneName == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	if len(regex) < 3 {
		fmt.Println("-name string should be 3 or more character long.")
		return
	}

	zoneID, err := getZoneID(client, zoneName)
	if err != nil {
		log.Fatal(err)
	}

	re := regexp.MustCompile(regex)
	isMatch := func(rrs *route53.ResourceRecordSet) bool {
		return rrs.Type != nil && rrs.Name != nil &&
			*rrs.Type == route53.RRTypeA && re.MatchString(*rrs.Name)
	}

	resourceRecordSets, err := getRecords(client, aws.String(zoneID), isMatch)
	if err != nil {
		log.Fatal(err)
	}

	if len(resourceRecordSets) == 0 {
		fmt.Println("There are no matching routes.")
		return
	}

	fmt.Println("Matching routes are:")
	for _, rrs := range resourceRecordSets {
		fmt.Printf("  -- %s\n", strings.Replace(*rrs.Name, `\052`, "*", 1))
	}

	delete := false
	for {
		fmt.Print("Are you sure you want delete those [yes/no]?")
		choice := ""
		fmt.Scanln(&choice)
		choice = strings.ToLower(choice)
		if choice == "yes" || choice == "no" {
			delete = choice == "yes"
			break
		}
	}
	if delete {
		fmt.Println("deleting...")
		err = deleteRecords(client, aws.String(zoneID), resourceRecordSets)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func getZoneID(client *route53.Route53, zoneName string) (string, error) {

	zones, err := client.ListHostedZonesByName(&route53.ListHostedZonesByNameInput{DNSName: &zoneName})
	if err != nil {
		return "", err
	}

	var zoneID *route53.HostedZone = nil
	for _, z := range zones.HostedZones {
		if z.Name != nil && *z.Name == zoneName {
			zoneID = z
			break
		}
	}

	if zoneID == nil || zoneID.Id == nil {
		return "", fmt.Errorf(fmt.Sprintf("Couldn't find %s zone.\n", zoneName))
	}
	return *zoneID.Id, nil
}

func deleteRecords(client *route53.Route53, zoneID *string, resourceRecordSets []*route53.ResourceRecordSet) error {

	changes := make([]*route53.Change, 0, 16)

	for _, rrs := range resourceRecordSets {
		change := &route53.Change{
			Action:            aws.String(route53.ChangeActionDelete),
			ResourceRecordSet: rrs,
		}
		changes = append(changes, change)
	}

	changeResourceRecordSetsInput := route53.ChangeResourceRecordSetsInput{
		HostedZoneId: zoneID,
		ChangeBatch:  &route53.ChangeBatch{Changes: changes},
	}

	_, err := client.ChangeResourceRecordSets(&changeResourceRecordSetsInput)

	// TODO we might want to wait until the deletion is completed,
	// but I think the delete is done almost instantaneously

	return err
}

func getRecords(client *route53.Route53, zoneID *string, isMatch func(*route53.ResourceRecordSet) bool) ([]*route53.ResourceRecordSet, error) {

	result := make([]*route53.ResourceRecordSet, 0, 16)

	onPage := func(page *route53.ListResourceRecordSetsOutput, isLastPage bool) bool {
		for _, rrs := range page.ResourceRecordSets {
			if isMatch(rrs) {
				result = append(result, rrs)
			}
		}
		return !isLastPage
	}

	err := client.ListResourceRecordSetsPages(&route53.ListResourceRecordSetsInput{HostedZoneId: zoneID}, onPage)
	if err != nil {
		return nil, err
	}

	return result, nil
}
