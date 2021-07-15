package smarthub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"

	"github.com/mxmCherry/openrtb/v15/openrtb2"
	"github.com/prebid/prebid-server/adapters"
	"github.com/prebid/prebid-server/config"
	"github.com/prebid/prebid-server/errortypes"
	"github.com/prebid/prebid-server/macros"
	"github.com/prebid/prebid-server/openrtb_ext"
)

const (
	ADAPTER_VER = "1.0.0"
)

type SmartHubAdapter struct {
	endpoint template.Template
}

type bidExt struct {
	MediaType string `json:"mediaType"`
}

func Builder(bidderName openrtb_ext.BidderName, config config.Adapter) (adapters.Bidder, error) {
	template, err := template.New("endpointTemplate").Parse(config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("unable to parse endpoint url template: %v", err)
	}

	bidder := &SmartHubAdapter{
		endpoint: *template,
	}

	return bidder, nil
}

func (a *SmartHubAdapter) buildEndpointURL(params *openrtb_ext.ExtSmartHub) (string, error) {
	endpointParams := macros.EndpointTemplateParams{Host: params.Host, AccountID: params.Seat, SourceId: params.Token}
	return macros.ResolveMacros(a.endpoint, endpointParams)
}

func (a *SmartHubAdapter) MakeRequests(
	openRTBRequest *openrtb2.BidRequest,
	reqInfo *adapters.ExtraRequestInfo,
) (
	requestsToBidder []*adapters.RequestData,
	errs []error,
) {
	var smarthubExt *openrtb_ext.ExtSmartHub

	reqJSON, err := json.Marshal(openRTBRequest)
	if err != nil {
		return nil, []error{err}
	}

	url, err := a.buildEndpointURL(smarthubExt)
	if err != nil {
		return nil, []error{err}
	}

	headers := http.Header{}
	headers.Add("Content-Type", "application/json;charset=utf-8")
	headers.Add("Accept", "application/json")
	headers.Add("Prebid-Adapter-Ver", ADAPTER_VER)

	return []*adapters.RequestData{{
		Method:  http.MethodPost,
		Body:    reqJSON,
		Uri:     url,
		Headers: headers,
	}}, nil
}

func (a *SmartHubAdapter) MakeBids(
	openRTBRequest *openrtb2.BidRequest,
	requestToBidder *adapters.RequestData,
	bidderRawResponse *adapters.ResponseData,
) (
	bidderResponse *adapters.BidderResponse,
	errs []error,
) {
	if bidderRawResponse.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if bidderRawResponse.StatusCode == http.StatusBadRequest {
		return nil, []error{&errortypes.BadInput{
			Message: fmt.Sprintf("Bad Request. %s", string(bidderRawResponse.Body)),
		}}
	}

	if bidderRawResponse.StatusCode == http.StatusServiceUnavailable {
		return nil, []error{&errortypes.BadInput{
			Message: "Bidder unavailable. Please contact the bidder support.",
		}}
	}

	if bidderRawResponse.StatusCode != http.StatusOK {
		return nil, []error{&errortypes.BadServerResponse{
			Message: fmt.Sprintf("Status Code: [ %d ] %s", bidderRawResponse.StatusCode, string(bidderRawResponse.Body)),
		}}
	}

	responseBody := bidderRawResponse.Body
	var bidResp openrtb2.BidResponse
	if err := json.Unmarshal(responseBody, &bidResp); err != nil {
		return nil, []error{err}
	}

	if len(bidResp.SeatBid) == 0 {
		return nil, []error{&errortypes.BadServerResponse{
			Message: "Empty SeatBid",
		}}
	}

	bidResponse := adapters.NewBidderResponseWithBidsCapacity(1)

	bids := bidResp.SeatBid[0].Bid

	if len(bids) == 0 {
		return nil, []error{&errortypes.BadServerResponse{
			Message: "Empty SeatBid.Bids",
		}}
	}

	bid := bids[0]

	var bidExt bidExt
	var bidType openrtb_ext.BidType

	if err := json.Unmarshal(bid.Ext, &bidExt); err != nil {
		return nil, []error{&errortypes.BadServerResponse{
			Message: "Missing BidExt",
		}}
	}

	bidType, err := getBidType(bidExt)

	if err != nil {
		return nil, []error{err}
	}

	bidResponse.Bids = append(bidResponse.Bids, &adapters.TypedBid{
		Bid:     &bid,
		BidType: bidType,
	})
	return bidResponse, nil
}

func getBidType(ext bidExt) (openrtb_ext.BidType, error) {
	return openrtb_ext.ParseBidType(ext.MediaType)
}
