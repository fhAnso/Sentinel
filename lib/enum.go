package lib

import (
	"Sentinel/lib/utils"
	"bufio"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func PassiveEnum(args *utils.Args, client *http.Client, filePaths *utils.FilePaths) {
	utils.GStdout.Flush()
	startTime := time.Now()
	utils.VerbosePrint("[*] Formatting db entries..\n")
	/*
		Read and format the entries listed in db.go, and if specified,
		also handle the endpoints indicated by the -x flag.
	*/
	endpoints, err := utils.EditDbEntries(args)
	if err != nil {
		utils.Glogger.Println(err)
	}
	utils.VerbosePrint("[*] Sending GET request to endpoints..\n")
	/*
		Send a GET request to each endpoint and filter the results. The results will
		be temporarily stored in the appropriate pool. Duplicates will be removed.
	*/
	for idx := 0; idx < len(endpoints); idx++ {
		if err := utils.EndpointRequest(client, args.Domain, endpoints[idx]); err != nil {
			utils.Glogger.Println(err)
		}
	}
	if len(utils.GPool.PoolSubdomains) == 0 {
		fmt.Fprintln(utils.GStdout, "[-] Could not determine subdomains :(")
		utils.GStdout.Flush()
		os.Exit(0)
	}
	var streams utils.FileStreams
	/*
		Specify the name and path for each output file. If all settings are configured, open
		separate file streams for each category (Subdomains, IPv4 addresses, and IPv6 addresses).
	*/
	if !args.DisableAllOutput {
		err = streams.OpenOutputFileStreams(filePaths)
		if err != nil {
			utils.Glogger.Println(err)
		}
		defer streams.CloseOutputFileStreams()
	}
	/*
		Iterate through the subdomain pool and process the current entry. The OutputHandler
		function will ensure that all fetched data is separated and stored within the output
		files, and it will also handle other actions specified by the command line.
	*/
	for _, subdomain := range utils.GPool.PoolSubdomains {
		paramsSetupFiles := utils.ParamsSetupFilesBase{
			FileParams: &utils.Params{},
			CliArgs:    args,
			FilePaths:  filePaths,
			Subdomain:  subdomain,
		}
		utils.ParamsSetupFiles(paramsSetupFiles)
		utils.OutputHandler(&streams, client, args, *paramsSetupFiles.FileParams)
	}
	poolSize := len(utils.GPool.PoolSubdomains)
	// Evaluate the summary and format it for writing to stdout.
	utils.Evaluation(startTime, poolSize)
}

func ActiveEnum(args *utils.Args, client *http.Client, filePaths *utils.FilePaths) {
	wordlistStream, entryCount := utils.WordlistStreamInit(args)
	defer wordlistStream.Close()
	scanner := bufio.NewScanner(wordlistStream)
	fmt.Fprintln(utils.GStdout)
	if !utils.GDisableAllOutput {
		utils.OpenOutputFileStreamsWrapper(filePaths)
		defer utils.GStreams.CloseOutputFileStreams()
	}
	for scanner.Scan() {
		utils.GSubdomBase = utils.SubdomainBase{}
		entry := scanner.Text()
		url := fmt.Sprintf("http://%s.%s", entry, args.Domain)
		statusCode := utils.HttpStatusCode(client, url)
		/*
			Skip failed GET requests and set the successful response subdomains to the
			Params struct. The OutputHandler function will ensure that all fetched data
			is separated and stored within the output files, and it will also handle
			other actions specified by the command line.
		*/
		if statusCode != -1 {
			subdomain := fmt.Sprintf("%s.%s", entry, args.Domain)
			paramsSetupFiles := utils.ParamsSetupFilesBase{
				FileParams: &utils.Params{},
				CliArgs:    args,
				FilePaths:  filePaths,
				Subdomain:  subdomain,
			}
			utils.ParamsSetupFiles(paramsSetupFiles)
			fmt.Fprint(utils.GStdout, "\r")
			utils.OutputHandler(&utils.GStreams, client, args, *paramsSetupFiles.FileParams)
			utils.GStdout.Flush()
			utils.GObtainedCounter++
		}
		utils.PrintProgress(entryCount)
	}
	utils.ScannerCheckError(scanner)
	fmt.Print("\r")
	utils.Evaluation(utils.GStartTime, utils.GObtainedCounter)
}

func DnsEnum(args *utils.Args, client *http.Client, filePaths *utils.FilePaths) {
	/*
		Ensure that the specified wordlist can be found and open
		a read-only stream.
	*/
	wordlistStream, entryCount := utils.WordlistStreamInit(args)
	defer wordlistStream.Close()
	if !utils.GDisableAllOutput {
		utils.OpenOutputFileStreamsWrapper(filePaths)
		defer utils.GStreams.CloseOutputFileStreams()
	}
	scanner := bufio.NewScanner(wordlistStream)
	fmt.Fprintln(utils.GStdout)
	/*
		Check if a custom DNS server address is specified by the -dnsC
		flag. If it is specified, ensure that the IP address follows the
		correct pattern and that the specified port is within the correct range.
	*/
	if args.DnsLookupCustom != "" {
		testValue := strings.Split(args.DnsLookupCustom, ":")
		dnsServerIp := net.ParseIP(testValue[0])
		if testValue == nil {
			utils.SentinelExit(utils.SentinelExitParams{
				ExitCode:    -1,
				ExitMessage: "Please specify a valid DNS server address",
				ExitError:   nil,
			})
		}
		dnsServerPort, err := strconv.ParseInt(testValue[1], 0, 16)
		if err != nil || dnsServerPort < 1 && dnsServerPort > 65535 {
			utils.SentinelExit(utils.SentinelExitParams{
				ExitCode:    -1,
				ExitMessage: "Please specify a valid DNS server port",
				ExitError:   nil,
			})
		}
		utils.CustomDnsServer = string(dnsServerIp)
	}
	for scanner.Scan() {
		utils.GDnsResults = []string{}
		entry := scanner.Text()
		subdomain := fmt.Sprintf("%s.%s", entry, args.Domain)
		utils.GDnsResolver = utils.DnsResolverInit(false)
		if utils.CustomDnsServer != "" {
			// Use custom DNS server address
			utils.GDnsResolver = utils.DnsResolverInit(true)
		}
		// Perform DNS lookup against the current subdomain
		utils.DnsLookups(utils.GDnsResolver, utils.DnsLookupOptions{
			IpAddress: nil,
			Subdomain: subdomain,
		})
		if len(utils.GDnsResults) != 0 {
			paramsSetupFiles := utils.ParamsSetupFilesBase{
				FileParams: &utils.Params{},
				CliArgs:    args,
				FilePaths:  filePaths,
				Subdomain:  subdomain,
			}
			utils.ParamsSetupFiles(paramsSetupFiles)
			fmt.Fprint(utils.GStdout, "\r")
			utils.OutputHandler(&utils.GStreams, client, args, *paramsSetupFiles.FileParams)
			utils.GStdout.Flush()
			utils.GObtainedCounter++
		}
		utils.PrintProgress(entryCount)
	}
	utils.ScannerCheckError(scanner)
	fmt.Print("\r")
	utils.Evaluation(utils.GStartTime, utils.GObtainedCounter)
}

func RDnsEnum(args *utils.Args) {
	ipFileStream := utils.IpFileStreamInit(args)
	scanner := bufio.NewScanner(ipFileStream)
	fmt.Fprintln(utils.GStdout)
	utils.GStdout.Flush()
	for scanner.Scan() {
		entry := scanner.Text()
		utils.GDnsResolver = utils.DnsResolverInit(false)
		if utils.CustomDnsServer != "" {
			// Use custom DNS server address
			utils.GDnsResolver = utils.DnsResolverInit(true)
		}
		// Perform DNS lookup against the current subdomain
		utils.DnsLookups(utils.GDnsResolver, utils.DnsLookupOptions{
			IpAddress: net.ParseIP(entry),
			Subdomain: "",
		})
		fmt.Fprintf(utils.GStdout, "[+] %s\n", entry)
		for idx := 0; idx < len(utils.GDnsResults); idx++ {
			fmt.Fprintf(utils.GStdout, " | %s\n", utils.GDnsResults[idx])
		}
		utils.GStdout.Flush()
	}
	utils.ScannerCheckError(scanner)
}
