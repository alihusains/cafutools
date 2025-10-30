package main
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)
const (
	EnvironmentProd    = "prod"
	EnvironmentStaging = "staging"
	PreCheck  string = "pre_check"
	PostCheck string = "post_check"
	CatalogProdUrl          = "https://catalog.cafu.app/api"
	MediaProdUrl            = "https://media.cafu.app/api"
	OperationalAssetProdUrl = "https://operational-assets.cafu.app/api"
	BatteryVerticalIDProd   = "3"
	ServicingVerticalIDProd = "5"
	TyreVerticalIDProd      = "4"
	PhotoCheckIDProd = 1
	NotesCheckIDProd = 2
	CatalogStagingUrl          = "https://catalog.global.staging.cafu.app/api"
	MediaStagingUrl            = "https://media.global.staging.cafu.app/api"
	OperationalAssetStagingUrl = "https://operational-assets.global.staging.cafu.app/api"
	BatteryVerticalIDStaging   = "22"
	ServicingVerticalIDStaging = "24"
	TyreVerticalIDStaging      = "23"
	PhotoCheckIDStaging = 34
	NotesCheckIDStaging = 35
)
type Product struct {
	ProductID int `json:"product_id"`
}
type PayloadBody struct {
	Products []Product `json:"products"`
}
type OpsAsset struct {
	ID uint `json:"id"`
}
type OpsAssetsResp struct {
	Data []OpsAsset `json:"data"`
}
type Check struct {
	CheckID    int    `json:"check_id"`
	Category   string `json:"category"`
	IsRequired bool   `json:"is_required"`
}
type MediaRelation struct {
	OwnerID      string `json:"owner_id"`
	OwnerType    string `json:"owner_type"`
	OwnerService string `json:"owner_service"`
}
type PipelineRunner struct {
	pool   chan struct{}
	client *http.Client
	CatalogUrl           string
	MediaUrl             string
	OperationalAssetsUrl string
	VariationIDs []int
	VerticalID   string
	PhotoCheckID int
	NotesCheckID int
	// MediaIDs is the media id of the catalog item which should be attached to all its variations
	MediaIDs []int
}
func NewPipeLineRunner(env string, verticalID string, variationIDs []int, mediaIDs []int) *PipelineRunner {
	if env == EnvironmentProd {
		return &PipelineRunner{
			pool:                 make(chan struct{}, 3),
			client:               &http.Client{},
			CatalogUrl:           CatalogProdUrl,
			MediaUrl:             MediaProdUrl,
			OperationalAssetsUrl: OperationalAssetProdUrl,
			VariationIDs:         variationIDs,
			VerticalID:           verticalID,
			PhotoCheckID:         PhotoCheckIDProd,
			NotesCheckID:         NotesCheckIDProd,
			MediaIDs:             mediaIDs,
		}
	}
	return &PipelineRunner{
		pool:                 make(chan struct{}, 3),
		client:               &http.Client{},
		CatalogUrl:           CatalogStagingUrl,
		MediaUrl:             MediaStagingUrl,
		OperationalAssetsUrl: OperationalAssetStagingUrl,
		VariationIDs:         variationIDs,
		VerticalID:           verticalID,
		PhotoCheckID:         PhotoCheckIDStaging,
		NotesCheckID:         NotesCheckIDStaging,
		MediaIDs:             mediaIDs,
	}
}
func (p *PipelineRunner) Add() {
	p.pool <- struct{}{}
}
func (p *PipelineRunner) Release() {
	<-p.pool
}
func (p *PipelineRunner) GetAssetIDs() []uint {
	serviceID := p.VerticalID
	baseUrl := p.OperationalAssetsUrl
	url := fmt.Sprintf("%s/v1/operational-assets/?service_ids=%s&page=1&per_page=100",
		baseUrl, serviceID)
	fmt.Println("url***\n", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("Error creating new http req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	fmt.Println("Response Status for the getOpsAssetIDs:", resp.Status)
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %v", err)
	}
	var opsAssetResp OpsAssetsResp
	err = json.Unmarshal(body, &opsAssetResp)
	if err != nil {
		log.Fatalf("Error unmarshaling JSON response: %v", err)
	}
	var assetIDs []uint
	for _, asset := range opsAssetResp.Data {
		assetIDs = append(assetIDs, asset.ID)
	}
	// Print the unmarshalled data
	// fmt.Println("AssetIDs are:", assetIDs)
	return assetIDs
}
func (p *PipelineRunner) MapAssetsToVariations(assetID string) {
	variationIDs := p.VariationIDs
	baseUrl := p.OperationalAssetsUrl
	fmt.Println("printing assetID", assetID)
	p.Add()
	go func() {
		defer p.Release()
		url := fmt.Sprintf("%s/v1/operational-assets/%s", baseUrl, assetID)
		fmt.Println("url***\n", url)
		var products []Product
		for _, variationID := range variationIDs {
			products = append(products, Product{ProductID: variationID})
		}
		payload := PayloadBody{Products: products}
		// Marshal the payload to JSON
		jsonData, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			log.Fatalf("Error marshaling payload to JSON: %v", err)
		}
		// Print or save the JSON payload to a file
		// fmt.Println(string(jsonData), "payload is")
		req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
		if err != nil {
			panic(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()
		fmt.Println("response Status:", resp.Status, assetID)
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			time.Sleep(30 * time.Second)
		}
	}()
}
func (p *PipelineRunner) MapChecksToVariations(variationID int) {
	fmt.Println("Printing variationID", variationID)
	baseUrl := p.CatalogUrl
	photo := p.PhotoCheckID
	notes := p.NotesCheckID
	checkIDs := []int{photo, notes}
	checks := []string{PreCheck, PostCheck}
	p.Add()
	go func() {
		defer p.Release()
		for _, checkStage := range checks {
			for _, checkID := range checkIDs {
				checkPayload := Check{
					CheckID:    checkID,
					Category:   checkStage,
					IsRequired: false,
				}
				if checkPayload.CheckID == photo {
					checkPayload.IsRequired = true
				}
				fmt.Println("Payload for checks mapping is", checkPayload)
				url := fmt.Sprintf("%s/v1/variations/%d/checks",
					baseUrl, variationID)
				fmt.Println("url***\n", url)
				// Marshal the payload to JSON
				jsonData, err := json.MarshalIndent(checkPayload, "", "  ")
				if err != nil {
					log.Fatalf("Error marshaling payload to JSON: %v", err)
					continue
				}
				// Print or save the JSON payload to a file
				fmt.Println("Payload is", string(jsonData))
				req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
				if err != nil {
					log.Printf("Error creating request: %v", err)
					continue
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := p.client.Do(req)
				if err != nil {
					log.Printf("Error sending request: %v", err)
					continue
				}
				defer resp.Body.Close()
				fmt.Printf("*** Response status: %s & variationID: %d ***\n", resp.Status, variationID)
				if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
					time.Sleep(30 * time.Second)
				}
			}
		}
	}()
}
func (p *PipelineRunner) MapMediaToVariations(variationID int) {
	fmt.Println("Printing variationID", variationID)
	variationIDStr := strconv.Itoa(variationID)
	baseUrl := p.MediaUrl
	mediaIDs := p.MediaIDs
	p.Add()
	go func() {
		defer p.Release()
		for _, mediaID := range mediaIDs {
			mediaPayload := MediaRelation{
				OwnerID:      variationIDStr,
				OwnerType:    "variations",
				OwnerService: "catalog",
			}
			fmt.Println("Payload for media mapping is", mediaPayload)
			url := fmt.Sprintf("%s/v1/media/%d/relations", baseUrl, mediaID)
			fmt.Println("url***\n", url)
			// Marshal the payload to JSON
			jsonData, err := json.MarshalIndent(mediaPayload, "", "  ")
			if err != nil {
				log.Fatalf("Error marshaling payload to JSON: %v", err)
				continue
			}
			// Print or save the JSON payload to a file
			fmt.Println("Payload is", string(jsonData))
			req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				log.Printf("Error creating request: %v", err)
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := p.client.Do(req)
			if err != nil {
				log.Printf("Error sending request: %v", err)
				continue
			}
			defer resp.Body.Close()
			fmt.Println("Response Status:", resp.Status, variationID)
			if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
				time.Sleep(30 * time.Second)
			}
		}
	}()
}
func RunOpsAsset(p *PipelineRunner) {
	opsAssetIDs := p.GetAssetIDs()
	fmt.Println("AssetIDs are", opsAssetIDs)
	fmt.Println("VariationIDs are", p.VariationIDs)
	if len(opsAssetIDs) == 0 {
		fmt.Println("Returning as no operational assetIDs present for given service_id")
		return
	}
	for i := 0; i < len(opsAssetIDs); i++ {
		p.MapAssetsToVariations(strconv.Itoa(int(opsAssetIDs[i])))
		if i%10 == 0 {
			time.Sleep(1 * time.Second)
		}
	}
	time.Sleep(10 * time.Second)
}
func RunPrePostChecks(p *PipelineRunner) {
	variationIDs := p.VariationIDs
	for i := 0; i < len(variationIDs); i++ {
		p.MapChecksToVariations(variationIDs[i])
		if i%10 == 0 {
			time.Sleep(1 * time.Second)
		}
	}
	time.Sleep(30 * time.Second)
}


func RunMediaRelations(p *PipelineRunner) {
	variationIDs := p.VariationIDs
	for i := 0; i < len(variationIDs); i++ {
		p.MapMediaToVariations(variationIDs[i])
		if i%10 == 0 {
			time.Sleep(1 * time.Second)
		}
	}
	time.Sleep(10 * time.Second)
}


func RunDeleteMediaRelations(p *PipelineRunner) {
	variationIDs := p.VariationIDs
	client := &http.Client{}
	// for i := 0; i < len(variationIDs); i++ {
		for _, variationID := range variationIDs {
		// baseUrl := p.MediaUrl
	

		getUrl := fmt.Sprintf("%s/v1/media?owner_service=catalog&owner_type=variations&owner_id=%d", p.MediaUrl, variationID)
		req, err := http.NewRequest("GET", getUrl, nil)
		if err != nil {
			log.Printf("Error creating GET request for variationID %d: %v", variationID, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error making GET request for variationID %d: %v", variationID, err)
			continue
		}
		defer resp.Body.Close()


		if resp.StatusCode != http.StatusOK {
			log.Printf("GET request for variationID %d failed with status: %d", variationID, resp.StatusCode)
			continue
		}

		// Parse the response to extract media IDs
		var apiResponse struct {
			Data []struct {
				Relation struct {
					MediaID int `json:"media_id"`
				} `json:"relation"`
			} `json:"data"`
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading GET response body for variationID %d: %v", variationID, err)
			continue
		}
		if err := json.Unmarshal(body, &apiResponse); err != nil {
			log.Printf("Error parsing GET response for variationID %d: %v", variationID, err)
			continue
		}

		for _, item := range apiResponse.Data {

			mediaID := item.Relation.MediaID
			deleteUrl := fmt.Sprintf("%s/v1/media/%d", p.MediaUrl, mediaID)
			deleteReq, err := http.NewRequest("DELETE", deleteUrl, nil)
			if err != nil {
				log.Printf("Error creating DELETE request for mediaID %d: %v", mediaID, err)
				continue
			}
			deleteReq.Header.Set("Content-Type", "application/json")

			deleteResp, err := client.Do(deleteReq)
			if err != nil {
				log.Printf("Error making DELETE request for mediaID %d: %v", mediaID, err)
				continue
			}
			defer deleteResp.Body.Close()

			if deleteResp.StatusCode != http.StatusOK {
				log.Printf("DELETE request for mediaID %d failed with status: %d", mediaID, deleteResp.StatusCode)
			} else {
				log.Printf("Successfully deleted mediaID %d", mediaID)
			}


		}



	}

}



/***
	DON'T NEED TO MODIFY ANYTHING ABOVE THIS LINE UNLESS TO CHANGE THE FUNCTIONALITY
***/
/*
Follow these steps to get the IDs list.
1. Get the item_id(s) from item_verticals based on the {Battery/Servicing/Tyre}VerticalID{Prod/Staging}
	SELECT
		STRING_AGG(item_id::text, ',') AS item_ids
	FROM
		item_verticals iv
	WHERE
		iv.vertical_id = 23; // Eg: Tyre vertical ID in staging.
2. Get the ids from variations table based on the item_id(s) from step 1.
	SELECT
		STRING_AGG(id::text, ',') AS ids
	FROM
		variations v
	WHERE
		v.item_id = 68; // Eg: Economy catalog item ID in prod.
*/

// ================================= PRODUCTION ============================================
// Get the latest list from DB and include all of them here (existing and newly added ones)
var BatteryVariationIDsProd = []int{
	4,86,76,2345,95,2233,2225,2344,2349,242,243,2212,236,235,72,82,239,628,2354,229,230,79,2350,97,2213,2228,2348,81,94,2353,96,240,2224,90,2226,74,75,84,85,2229,2232,2236,2220,2230,2223,2221,2218,2235,2240,2347,2351,87,92,2217,73,2237,237,233,2234,89,77,227,2222,232,91,88,238,2231,2238,228,80,2239,2227,78,98,2215,2216,93,2346,234,83,241,2352,2219,2214,231,244,245,254,249,250,248,255,2252,110,119,246,247,103,115,2245,104,105,109,2242,2246,2249,99,100,101,2241,2244,2248,113,116,106,108,112,2243,2247,117,252,253,2251,107,251,2250,629,111,114,118,102,2256,2258,120,634,2253,2259,2260,121,123,122,258,2255,256,631,257,632,633,4291,2257,2254,630,

}
// Get the latest list from DB and include all of them here (existing and newly added ones)
var ServicingVariationIDsProd = []int{
	5, 277, 330, 335, 309, 306, 278, 265, 635, 313, 261, 272, 318, 304, 326, 308, 298, 289, 266, 279, 636, 637, 638, 639, 286, 640, 641, 642, 643, 644, 645, 646, 262, 294, 333, 259, 297, 273, 287, 291, 267, 268, 311, 320, 303, 302, 269, 327, 263, 270, 288, 324, 328, 321, 284, 305, 332, 299, 283, 276, 312, 331, 281, 292, 317, 271, 310, 316, 300, 323, 280, 295, 325, 275, 285, 260, 264, 274, 282, 293, 296, 301, 307, 314, 315, 319, 322, 329, 334, 352, 343, 409, 340, 375, 370, 374, 360, 388, 402, 347, 358, 383, 366, 394, 408, 345, 372, 364, 381, 346, 350, 336, 411, 341, 413, 365, 361, 391, 362, 344, 390, 399, 342, 378, 368, 357, 380, 385, 377, 367, 369, 389, 338, 387, 354, 404, 401, 403, 410, 647, 648, 649, 650, 651, 652, 653, 654, 655, 656, 657, 353, 355, 373, 356, 359, 398, 371, 400, 406, 363, 376, 379, 382, 384, 386, 392, 393, 395, 397, 405, 407, 412, 337, 339, 348, 349, 351, 502, 486, 503, 493, 495, 456, 446, 488, 483, 416, 470, 474, 429, 463, 454, 435, 462, 508, 439, 432, 428, 423, 482, 458, 448, 440, 491, 455, 500, 441, 420, 426, 461, 442, 419, 467, 453, 449, 436, 477, 468, 460, 443, 465, 425, 444, 499, 434, 485, 418, 430, 479, 494, 505, 489, 414, 451, 498, 658, 659, 660, 661, 662, 663, 664, 665, 666, 667, 668, 669, 670, 671, 672, 673, 674, 675, 676, 677, 678, 679, 680, 681, 415, 417, 421, 422, 424, 427, 431, 433, 437, 438, 445, 447, 450, 452, 457, 459, 464, 466, 471, 472, 475, 476, 478, 480, 481, 484, 487, 490, 492, 496, 497, 501, 506, 507, 509,
}
// Get the latest list from DB and include all of them here (existing and newly added ones)
var TyreVariationIDsProd = []int{
	5696,5697,5698,5699,5700,5701,5702,5703,5704,5705,5706,5707,5708,5709,5710,5711,5712,5713,5714,5715,5716,5717,5718,5719,5720,5721,5722,5723,5724,5725,5726,5727,5728,5729,5730,5731,5732,5733,5734,5735,5736,5737,5738,5739,5740,5741,5742,5743,5744,5745,5746,5747,5748,5749,5750,5751,5752,5753,5754,5755,5756,5757,5758,5759,5760,5761,5762,5763,5764,5765,5766,5767,5768,5769,5770,5771,5772,5773,5774,5775,5776,5777,5778,5779,5780,5781,5782,5783,5784,5785,5786,5787,5788,5789,5790,5791,5792,5793,5794,5795,5796,5797,5798,5799,5800,5801,5802,5803,5804,5805,5806,5807,5808,5809,5810,5811,5812,5813,5814,5815,5816,5817,5818,5819,5820,5821,5822,5823,5824,5825,5826,5827,5828,5829,5830,5831,5832,5833,5834,5835,5836,5837,5838,5839,5840,5841,5842,5843,5844,5845,5846,5847,5848,5849,5850,5851,5852,5853,5854,5855,5856,5857,5858,5859,5860,5861,5862,5863,5864,5865,5866,5867,5868,5869,5870,5871,5872,5873,5874,5875,5876,5877,5878,5879,5880,5881,5882,5883,5884,4588,4589,4595,4598,4599,4765,4766,4767,4768,4769,4770,4771,4772,4773,4774,4775,4776,4777,4778,4779,4780,4781,4782,4783,4784,4785,4786,4787,4788,4789,4790,4791,4792,4793,4794,4795,4796,4797,4798,4799,4800,4801,4802,4803,4804,4805,4806,4807,4808,4809,4810,4811,4812,4813,4814,4815,4816,4817,4818,4819,4820,4821,4822,4823,4824,4825,4826,4827,4828,4829,4830,4831,4832,4833,4834,4835,4836,4837,4838,4839,4840,4841,4842,4843,4844,4845,4846,4847,4848,4849,4850,4851,4852,4853,4854,4855,4856,4857,4858,4859,4860,4861,4862,4863,4864,4865,4866,4867,4868,4869,4870,4871,4872,4873,4874,4875,4876,4877,4878,4879,4880,4881,4882,4883,4884,4590,4591,4592,4593,4594,4885,4886,4887,4888,4889,4890,4891,4892,4893,4894,4895,4896,4897,4898,4899,4900,4901,4902,4903,4904,4905,4906,4907,4908,4909,4910,4911,4912,4913,4914,4915,4916,4917,4918,4919,4920,4921,4922,4923,4924,4925,4926,4927,4928,4929,4930,4931,4932,4933,4934,4935,4936,4937,4938,4939,4940,4941,4942,4943,4944,4945,4946,4947,4948,4949,4950,4951,4952,4953,4954,4955,4956,4957,4958,4959,4960,4961,4962,4963,4964,4965,4966,4967,4968,4969,4970,4971,4972,4973,4974,4975,4976,4977,4978,4979,4980,4981,4982,4983,4984,4985,4986,4987,4988,4989,4990,4991,4992,4993,4994,4995,4996,4997,4998,4999,5000,5001,5002,5003,5004,5005,5006,5007,5008,5009,5010,5011,5012,5013,5014,5015,5016,5017,5018,5019,5020,5021,5022,5023,5024,5025,5026,5027,5028,5029,5030,5031,5032,5033,5034,5035,5036,5037,5038,5039,5126,5127,5040,5041,5042,5043,5044,5045,5046,5047,5048,5049,5050,5051,5052,5053,5054,5055,5056,5057,5058,5059,5060,5061,5062,5063,5064,5065,5066,5067,5068,5069,5070,5071,5072,5073,5074,5075,5076,5077,5078,5079,5080,5081,5082,5083,5084,5085,5086,5087,5088,5089,5090,5091,5092,5093,5094,5095,5097,5098,5099,5100,5101,5102,5103,5104,5105,5106,5107,5108,5109,5110,5111,5112,5113,5114,5115,5116,4596,4597,4600,4601,4602,4603,4604,4605,5117,4606,4607,4608,4609,4610,4611,4612,4613,4614,4615,4616,4617,5096,4618,4619,4620,4621,4622,4623,4624,4625,4626,4627,4628,4629,4630,4631,4632,4633,4634,4635,4636,4637,4638,4639,4640,4641,4642,4643,4644,4645,4646,4647,4648,4649,4650,4651,4652,4653,4654,4655,4656,4657,4658,4659,4660,4661,4662,4663,4664,4665,4666,4667,5118,4668,4669,4670,4671,4672,4673,4674,4675,4676,4677,4678,4679,4680,4681,4682,4683,4684,4685,4686,4687,4688,4689,4690,4691,4692,4693,4694,4695,4696,4697,4698,4699,4700,4701,4702,4703,4704,4705,4706,4707,4708,4709,4710,4711,4712,4713,4714,4715,4716,4717,4718,4719,4720,4721,4722,4723,4724,4725,4726,4727,4728,4729,4730,4731,4732,4733,4734,4735,4736,4737,4738,4739,4740,4741,4742,4743,4744,4745,4746,4747,4748,4749,4750,4751,4752,4753,4754,4755,4756,4757,4758,4759,4760,4761,4762,4763,4764,5119,5120,5121,5122,5123,5124,5125,5128,5129,5130,5131,5132,5133,5134,5135,5136,5137,5138,5139,5140,5141,5142,5143,5144,5145,5146,5147,5148,5149,5150,5151,5152,5153,5154,5155,5156,5157,5158,5159,5160,5161,5162,5163,5164,5165,5166,5167,5168,5169,5170,5171,5172,5173,5174,5175,5176,5177,5178,5179,5180,5181,5182,5183,5184,5185,5186,5187,5188,5189,5190,5191,5192,5193,5194,5195,5196,5197,5198,5199,5200,5201,5202,5203,5204,5205,5206,5207,5208,5209,5210,5211,5212,5213,5214,5215,5216,5217,5218,5219,5220,5221,5222,5223,5224,5225,5226,5227,5228,5229,5230,5231,5232,5233,5234,5235,5236,5237,5238,5239,5240,5241,5242,5243,5244,5245,5246,5247,5248,5249,5250,5251,5252,5253,5254,5255,5256,5257,5258,5259,5260,5261,5262,5263,5264,5265,5266,5267,5268,5269,5270,5271,5272,5273,5274,5275,5276,5277,5278,5279,5280,5281,5282,5283,5284,5285,5286,5287,5288,5289,5290,5291,5292,5293,5294,5295,5296,5297,5298,5299,5300,5301,5302,5303,5304,5305,5306,5307,5308,5309,5310,5311,5312,5313,5314,5315,5316,5317,5318,5319,5320,5321,5322,5323,5324,5325,5326,5327,5328,5329,5330,5331,5332,5333,5334,5335,5336,5337,5338,5339,5340,5341,5342,5343,5344,5345,5346,5347,5348,5349,5350,5351,5352,5353,5354,5355,5356,5357,5358,5359,5360,5361,5362,5363,5364,5365,5366,5367,5368,5369,5370,5371,5372,5373,5374,5375,5376,5377,5378,5379,5380,5381,5382,5383,5384,5385,5386,5387,5388,5389,5390,5391,5392,5393,5394,5395,5396,5397,5398,5399,5400,5401,5402,5403,5404,5405,5406,5407,5408,5409,5410,5411,5412,5413,5414,5415,5416,5417,5418,5419,5420,5421,5422,5423,5424,5425,5426,5427,5428,5429,5430,5431,5432,5433,5434,5435,5436,5437,5438,5439,5440,5441,5442,5443,5444,5445,5446,5447,5448,5449,5450,5451,5452,5453,5454,5455,5456,5457,5458,5459,5460,5461,5462,5463,5464,5465,5466,5467,5468,5469,5470,5471,5472,5473,5474,5475,5476,5477,5478,5479,5480,5481,5482,5483,5484,5485,5486,5487,5488,5489,5490,5491,5492,5493,5494,5495,5512,5513,5514,5515,5516,5517,5518,5519,5520,5521,5522,5523,5524,5525,5526,5527,5528,5529,5530,5531,5532,5533,5534,5535,5536,5537,5538,5539,5540,5541,5542,5543,5544,5545,5546,5547,5548,5549,5550,5551,5552,5553,5554,5555,5556,5557,5558,5559,5560,5561,5562,5563,5564,5565,5566,5567,5568,5569,5570,5571,5572,5573,5574,5575,5576,5577,5578,5579,5580,5581,5582,5583,5584,5585,5586,5587,5588,5589,5590,5591,5592,5593,5594,5595,5596,5597,5598,5599,5600,5601,5602,5603,5604,5605,5606,5607,5608,5609,5610,5611,5612,5613,5614,5615,5616,5617,5618,5619,5620,5621,5622,5623,5624,5625,5626,5627,5628,5629,5630,5631,5632,5633,5634,5635,5636,5637,5638,5639,5640,5641,5642,5643,5644,5645,5646,5647,5648,5649,5650,5651,5652,5653,5654,5655,5656,5657,5658,5659,5660,5661,5662,5663,5664,5665,5666,5667,5668,5669,5670,5671,5672,5673,5674,5675,5676,5677,5678,5679,5680,5681,5682,5683,5684,5685,5686,5687,5688,5689,5690,5691,5692,5693,5694,5695,6,

}


// ================================ STAGING ================================
// Get the latest list from DB and include all of them here (existing and newly added ones)
var BatteryVariationIDsStaging = []int{
	13198, 13194, 13212, 13206, 13674, 13671, 13684, 17292, 13203, 13190, 13202, 13208, 13191, 13201, 13213, 13681, 13676, 13679, 13205, 13207, 13193, 13200, 13204, 13211, 13680, 13672, 13188, 13210, 13195, 13196, 13214, 13685, 13192, 13669, 13670, 13682, 13673, 13677, 13675, 13678, 13199, 13189, 13683, 13197, 13209,
}
// Get the latest list from DB and include all of them here (existing and newly added ones)
var ServicingVariationIDsStaging = []int{
	13708, 13709, 13710, 13711, 13712, 13713, 13714, 13715, 13716, 13717, 13718, 13719, 13720, 13721, 13722, 13723, 13724, 13725, 13726, 13727, 13728, 13729, 13730, 13731, 13732, 13733, 13734, 13735, 13736, 13737, 13738, 13739, 13740, 13741, 13742, 13743, 13744, 13745, 13746, 13747, 13748, 13749, 13750, 13751, 13752, 13753, 13754, 13755, 13756, 13757, 13758, 13759, 13760, 13761, 13762, 13763, 13764, 13765, 13766, 13767, 13768, 13769, 13770, 13771, 13772, 13773, 13774, 13775, 13776, 13777, 13778, 13779, 13780, 13781, 13782, 13783, 13784, 13785, 13786, 13787, 13788, 13789, 13790, 13791, 13792, 13793, 13794, 13795, 13796, 13797, 13798, 13799, 13800, 13801, 13802, 13803, 13804, 13805, 13806, 13807, 13808, 13809, 13810, 13811, 13812, 13813, 13814, 13815, 13816, 13817, 13818, 13819, 13820, 13821, 13822, 13823, 13824, 13825, 13826, 13827, 13828, 13829, 13830, 13831, 13832, 13833, 13834, 13835, 13836, 13837, 13838, 13839, 13840, 13841, 13842, 13843, 13844, 13845, 13846, 13847, 13848, 13849, 13850, 13851, 13852, 13853, 13854, 13855, 13856, 13857, 13858, 13859, 13860, 13861, 13862, 13863, 13864, 13865, 13866, 13867, 13868, 13869, 13870, 13871, 13872, 13873, 13874, 13875, 13876, 13877, 13878, 13879, 13880, 13881, 13882, 13883, 13884, 13885, 13886, 13887, 13888, 13889, 13890, 13891, 13892, 13893, 13894, 13895, 13896, 13897, 13898, 13899, 13900, 13901, 13902, 13903, 13904, 13905, 13906, 13907, 13908, 13909, 13910, 13911, 13912, 13913, 13914, 13915, 13916, 13917, 13918, 13919, 13920, 13921, 13922, 13923, 13924, 13925, 13926, 13927, 13928, 13929, 13930, 13931, 13932, 13933, 13934, 13935, 13936, 13937, 13938, 13939, 13940, 13941, 13942, 13943, 13944, 13945, 13946, 13947, 13948, 13949, 13950, 13951, 13952, 13953, 13954, 13955, 13956, 13957, 13958, 14036, 17254, 17255, 17256, 17257, 17258, 17259, 17260, 17261, 17262, 17263, 17264, 17265, 17266, 17267, 17268, 17269, 17270, 17271, 17272, 17273, 17274, 17275, 17276, 17277, 17278, 17279, 17280, 17281, 17282, 17283, 17284, 17285, 17286, 17287, 17288, 17289, 17290, 17291,
}
// Get the latest list from DB and include all of them here (existing and newly added ones)
var TyreVariationIDsStaging = []int{
24551,25300,24748,25304,24749,25045,25306,25309,24754,24755,25311,24756,24558,25312,24759,25313,25314,24627,24760,25316,24761,24762,24641,25317,24763,24552,24770,25322,24462,24771,25324,25325,25328,24796,25330,25331,24742,24799,24774,24730,25336,25105,24775,25337,25343,24778,25345,24779,24780,25348,24781,25350,25351,24792,24794,25352,24797,24801,25353,25358,25361,24803,24809,25364,24299,25368,24824,24469,24811,25371,24812,25374,25377,25378,24815,25379,24820,25380,24822,25381,25382,25383,24829,25384,25385,25389,25391,25393,25394,25395,25397,25401,25402,25405,25407,25408,25409,25410,25412,25413,24844,25414,24793,25415,25406,24847,24848,25418,25419,24900,25420,24707,24850,24852,25421,25422,24587,24922,25423,24853,25425,25426,24854,24386,25428,25432,24786,24954,25433,25434,25435,24977,25436,25437,25046,25438,25441,25048,24859,24828,24980,25442,25443,25444,25049,25445,25446,24689,25448,25449,24860,25015,24582,25450,24861,25022,25451,25453,24506,25030,25454,25455,25031,25456,24862,24271,25055,24272,25457,24776,25458,24863,25460,24864,25461,25462,24867,24276,25075,25463,24300,25051,24281,25464,24311,25466,25467,24315,24875,24283,25056,25468,25469,24344,25472,24373,25473,24390,25476,24396,24830,24869,25481,25482,24404,25059,25484,24428,25485,24290,24880,25488,24291,24884,25490,24838,25073,24292,25492,24795,24876,25076,25497,24293,24833,25502,24294,24885,25503,24430,25504,25064,24440,24888,25505,24791,24296,25065,24297,25506,24890,25511,25512,24452,25513,24456,25514,24461,24895,25515,25518,25519,24479,25520,24483,24905,25521,24301,24485,24491,24493,25524,24805,24784,24501,25525,25087,24502,25527,24304,24908,25528,24305,25099,24503,24910,25529,25500,24504,25092,24505,25100,24306,24521,25530,25101,25532,24915,24307,25533,25534,25095,24507,25535,24510,25536,24511,24512,25537,25540,24513,25123,25543,24298,25544,24274,24817,25545,25548,24924,24514,24925,24359,24818,25104,24517,24927,24518,25134,25139,24519,24520,25110,25160,24522,24318,24823,25111,24523,24524,24320,25172,24321,24938,24282,25174,25176,24528,24529,24941,25180,24325,24525,24530,25074,24531,25335,24946,24328,24947,24533,24534,25196,24535,24331,24960,24332,25210,25212,24284,24964,25342,24970,24336,24339,25217,24538,24546,24340,24979,24553,24981,24286,24982,24343,24556,24985,24557,24569,24571,24842,25133,24287,25122,24572,24989,24348,24349,24990,24991,24289,24368,25136,25229,25232,24351,24808,24575,25137,24353,25218,24994,25233,24995,24354,24355,24996,24849,24998,24356,24999,25000,24576,24579,25141,24583,25001,24360,25002,24361,25003,24303,25004,24586,25241,24589,24591,24973,24310,25131,25242,24316,25008,24598,24606,25447,25069,24612,24370,24813,25013,24926,25307,24372,25016,24543,25017,25252,24374,24613,25219,24516,25253,25157,25020,24326,25021,24376,24554,25254,24903,24377,24313,25023,25024,25026,25257,24870,24871,24378,25027,24623,24379,24617,25029,24358,25167,24873,25032,24327,24382,25014,25035,25036,24921,25338,24383,24329,25258,24333,24765,24631,24634,25260,24337,24490,24654,24657,25173,25041,24338,24660,25043,24662,24342,25262,25044,24585,25018,24389,25047,25050,24346,24671,24667,24393,24309,25054,24673,24394,25341,25057,25181,24486,24893,24347,25269,24856,24397,25058,24675,25126,24398,24697,24687,25183,24702,24399,25063,24703,24401,25066,24402,25067,24709,25068,25270,24711,25185,24715,25271,25077,24721,24723,24724,24725,24352,25085,24407,25086,25274,24334,24408,25088,24357,24729,24726,24731,24411,25093,24412,24733,24732,25096,24413,24415,25097,25098,24416,25102,24418,24736,24419,25284,24363,25292,24738,24420,24904,25424,25107,24365,24295,25108,24740,25112,25053,24423,25295,24424,25113,24741,24425,25116,24426,25117,86,25329,24744,25119,24367,24961,24745,24746,25089,24751,25340,24752,24753,24384,25301,24527,25124,25127,24434,24764,25129,24436,24766,24767,25138,24976,24441,25200,24772,24773,24782,25142,24392,24446,25143,24783,25145,24448,24788,24449,24787,24450,24800,24395,24790,25150,24802,24804,24453,25152,25154,24458,25158,24459,24992,25305,24391,25159,24806,25211,24807,25310,25161,25315,24463,24405,24810,24816,24465,24821,24819,24406,25215,24568,24825,24826,25319,25166,24409,24827,25169,24421,24422,24937,25323,24831,24832,25171,24555,24834,24837,24484,24836,25222,24839,25326,24548,24840,24841,24474,25177,24843,24432,25239,24845,24482,25182,25388,24897,24435,25184,24851,24477,24846,25363,25234,24855,25187,24478,25010,24438,25188,24857,25327,24858,24865,24866,24868,24945,24487,25193,24874,25332,24387,24872,24489,25194,24532,25195,24877,24492,25339,25199,24878,25344,24879,24881,25346,24495,24496,25204,25205,24642,25249,24882,25156,24883,25206,24886,24317,24500,25207,24887,24891,25255,24966,24481,24892,24894,25019,24896,24898,24899,24901,24902,25264,24906,24909,24911,24498,25226,25273,24907,24984,24912,24913,24916,24918,24920,24923,24928,24929,24930,24464,25354,24931,24270,24273,25240,24935,24275,24277,24278,24279,25243,24280,25355,24933,24285,25362,25244,24288,25245,24308,25365,24312,25367,24670,24314,24319,25248,24324,24934,24936,24330,25251,24350,24362,24364,24366,24371,24375,24380,25261,24381,24385,25263,25369,24388,24400,24414,25267,24417,25268,24427,24429,24431,25370,24433,25179,24437,25357,24439,25272,25286,24442,24919,24443,25275,24444,25372,24445,24940,24942,24447,25278,24451,24454,24943,24455,25281,24540,25282,25283,24541,24457,24460,24466,25373,25285,24468,25375,24944,24948,24470,25288,24471,24472,25289,24949,24473,24369,25318,24475,25320,24480,25321,24488,25290,24494,24497,24499,24508,25291,24950,24951,24953,24955,24956,24957,24959,24962,24526,24965,24967,24536,24537,24969,24971,24539,24542,25396,24559,24544,24972,24974,24545,25398,25347,24547,24917,24975,24549,24983,24986,25011,25403,25404,24550,25296,24560,25416,24564,24561,24987,24988,24562,24563,25349,24567,24566,25356,25360,25334,25359,25298,24785,24509,24570,24573,25417,24789,24574,25366,25429,24600,25431,24577,25439,24578,25440,24580,25376,24302,24592,25302,24584,25386,24608,25387,25390,24611,25303,25452,24588,25399,25400,25411,24590,24997,25427,25308,24593,24594,25005,25006,25007,24595,25009,25012,25025,25028,25033,25038,25039,24596,25040,25042,25052,25060,25061,25465,25062,25070,25474,24597,25071,25072,25078,25079,25080,25081,24626,25082,25083,25084,25090,25091,25094,25103,25106,24599,24601,25109,25114,25115,25118,25121,25499,25507,24602,24603,25130,24747,25125,25128,25508,25509,25148,24604,25132,25135,25140,25144,25146,25147,25149,24605,25151,25153,25155,24607,25162,25163,24322,25459,25164,25165,25170,24609,24777,25168,24610,24614,24616,25175,25178,24618,24619,25392,24620,24403,25186,25189,24621,24622,24625,25190,25191,24628,25192,25197,25198,24629,25201,25202,25203,25208,25209,25213,25214,25216,25220,24630,25221,25223,25224,25225,25498,25227,25228,24565,25470,24639,25471,24647,24650,24655,25230,25231,24668,25236,25237,24581,24669,25526,25475,24632,24682,24633,25259,25120,24341,25246,24635,25238,24672,24674,24686,24636,25477,24637,25478,24467,24683,25479,25250,25480,24638,25247,25256,24688,25483,24690,25538,24691,24693,25486,25487,24963,24643,25333,25430,25542,24644,24694,25265,24695,25266,25276,25277,25489,24835,24345,24798,24720,24323,25235,24515,24814,25280,25279,24939,24476,24914,24645,24889,25034,25491,24410,24646,24696,24648,24684,24335,24649,24698,25037,24705,24640,24692,24704,24708,25287,25293,25294,25541,24651,24624,24714,24717,24652,24716,25297,25299,24718,24615,24653,24727,24728,24656,25493,25494,24734,24735,24932,25495,25496,25501,24952,25510,25516,25517,24958,24968,25522,25523,24978,25531,25539,24658,24993,24659,24661,25546,24663,25547,25549,25550,24664,24665,24666,24676,24677,24678,24679,24680,24681,24685,24699,24700,24701,24706,24710,24712,24713,24719,24722,24737,24739,24743,24750,24757,24758,24768,24769,
}
func main() {
	// Define the MediaID here. Irs required only when executing "RunMediaRelations()"
	mediaIDs := []int{}
	p := NewPipeLineRunner(
		// Define the environment
		EnvironmentProd,
		// EnvironmentStaging,
		// Define the environment specific vertical ID
		TyreVerticalIDProd,
		// BatteryVerticalIDProd,
		// BatteryVariationIDsProd,
		// TyreVerticalIDStaging,
		// Define the environment specific variation ID
		// TyreVariationIDsStaging,
		// BatteryVariationIDsStaging,
		// Define the environment specific variation ID
		TyreVariationIDsProd,
		mediaIDs,
	)
	// Uncomment only the required function calls and execute the script. Prefer to do one at a time and verify.
	// Attaching variation to op assets
	RunOpsAsset(p)
	// Attaching checks to variations
	RunPrePostChecks(p)
	// Attach the media relationships
	/*RunMediaRelations(p)*/


	// RunDeleteMediaRelations(p)
}



/* ============================= SEQUENCE TO RUN THIS SCRIPT ========================== //


 1. Upload the sheet using following API : 

curl --location 'https://internal-bff.global.staging.cafu.app/api/v1/catalog/upload-file' \
--header 'Accept: application/json' \
--header 'Authorization: Bearer eyJraWQiOiIwOHZFMks4Z0g4NXZyUXBQa29JVUZVWmtLYWpaVjluUnBVM3o5TFFPZW1nPSIsImFsZyI6IlJTMjU2In0.eyJzdWIiOiJjM2Q3MDQ2MS0yNDBmLTQ3NTUtODJiOC1hZTkxYjYzNTlhNmEiLCJjb2duaXRvOmdyb3VwcyI6WyJldS13ZXN0LTFfejU1UVZMRlM1X2F6dXJlQUQtaWRlbnRpdHktcHJvdmlkZXIiXSwiaXNzIjoiaHR0cHM6XC9cL2NvZ25pdG8taWRwLmV1LXdlc3QtMS5hbWF6b25hd3MuY29tXC9ldS13ZXN0LTFfejU1UVZMRlM1IiwidmVyc2lvbiI6MiwiY2xpZW50X2lkIjoiNjhrbzRqYTQ1NW9uMGNmc2JlN211Z3IwazkiLCJvcmlnaW5fanRpIjoiNmRkN2NkN2ItNjQ2OS00ZjEwLWIzZjItYTk0MDVlZDNiMGU2IiwidG9rZW5fdXNlIjoiYWNjZXNzIiwic2NvcGUiOiJhd3MuY29nbml0by5zaWduaW4udXNlci5hZG1pbiBwaG9uZSBvcGVuaWQgZW1haWwiLCJhdXRoX3RpbWUiOjE3MzI2MTE2NjYsImV4cCI6MTczMjYyMDQxOSwiaWF0IjoxNzMyNjE2ODE5LCJqdGkiOiIzMmY5ZGY3NC1lZTlkLTQ3MWMtODIzYi0xYTIwNzY2NDliYzEiLCJ1c2VybmFtZSI6ImF6dXJlYWQtaWRlbnRpdHktcHJvdmlkZXJfYWxpLnNvcmF0aGl5YUBjYWZ1LmNvbSJ9.S4m6JBc9XQBxYbEaI63NhuK8iuGZTvc91IEOq-g25eY0C8zjFdMR0jU0fjXlGX6DDLpH92slNDcIItcct0NeUx6xVGINMTk-4HR0KprAd8f6IwIyE2jn1KLGxKUFQxEeNimhLSCgH6kAKplCy19Z2_rK0ZdeCs1luusFS8AqTJrgFs1C6kV9y_hFxFHt1cXyuIpx7IYr_FPONqDgkJG73BuSCF6lRxen4gTzhCLSDhdrNIaYG5rucOxorh0glASPNoFoIswi5mL9ELThUOuza4ItGVVtUHUCxaOuXKTvcYGGxs3z6j_88GDx7XfT8VB1lKnp3Z3uNjNBthV30g0GZw' \
--form 'file=@"/Users/alihusainsorathiya/Desktop/Sraging 26 nov.csv"' \
--form 'vertical="tyre"' \
--form 'file_type="tyres"'


2. Check and monitor for errors on temporal
3. If the upload successful, Run this location API script using this tool [Make sure VPN is on]:  https://alihusains.github.io/cafutools/catalog/
4. From the DB, update the variations list in the script eg: in TyreVariationIDsStaging add all the updated variations of tyres for staging environment
4. Run OpsAsset(p) function from the script
5. Run prePostChecks(p) function from the script





Refer to this documentation : https://cafu.atlassian.net/wiki/spaces/CafuTech/pages/3677224962/BOT+-+Uploading+catalog+variations+using+producer+s+CSV+files

============================= END ========================== //	 */


