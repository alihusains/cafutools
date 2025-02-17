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
	595, 596, 597, 598, 599, 600, 601, 695, 694, 696, 697, 698, 699, 700, 701, 702, 602, 703, 603, 604, 704, 705, 706, 707, 708, 709, 710, 711, 712, 713, 714, 715, 716, 717, 718, 719, 720, 721, 722, 723, 724, 725, 726, 727, 728, 729, 730, 731, 732, 733, 734, 735, 736, 737, 738, 739, 740, 741, 742, 743, 744, 745, 746, 747, 748, 749, 750, 751, 752, 753, 754, 755, 756, 757, 758, 759, 760, 761, 762, 763, 764, 765, 766, 767, 768, 769, 770, 771, 772, 773, 774, 775, 776, 777, 778, 779, 780, 781, 782, 783, 784, 785, 786, 787, 788, 789, 790, 791, 792, 793, 794, 795, 796, 797, 798, 799, 800, 801, 802, 803, 804, 805, 807, 6, 806, 808, 809, 810, 811, 812, 813, 814, 815, 816, 817, 818, 819, 820, 821, 822, 823, 824, 825, 826, 827, 828, 829, 830, 831, 832, 833, 834, 835, 836, 837, 838, 839, 840, 841, 842, 843, 844, 845, 846, 847, 848, 849, 850, 851, 852, 853, 854, 855, 856, 857, 858, 859, 860, 861, 862, 863, 864, 865, 866, 867, 868, 869, 870, 871, 872, 873, 874, 875, 876, 877, 878, 879, 880, 881, 882, 883, 884, 885, 886, 887, 888, 889, 890, 891, 892, 893, 894, 895, 896, 897, 898, 899, 900, 901, 902, 903, 904, 905, 906, 907, 908, 909, 910, 911, 912, 913, 914, 915, 916, 917, 918, 919, 920, 921, 922, 923, 924, 925, 926, 927, 928, 929, 930, 931, 932, 933, 934, 935, 936, 937, 938, 939, 940, 941, 942, 943, 944, 945, 946, 947, 948, 949, 950, 951, 952, 953, 954, 955, 956, 957, 958, 959, 960, 961, 962, 963, 964, 965, 966, 967, 968, 969, 970, 971, 972, 973, 974, 975, 976, 977, 978, 979, 980, 981, 982, 983, 984, 985, 986, 987, 988, 989, 990, 991, 992, 993, 994, 995, 996, 997, 998, 999, 1000, 1001, 1002, 1003, 1004, 1005, 1006, 1007, 1008, 1009, 1010, 1011, 1012, 1013, 1014, 1015, 1016, 1017, 1018, 1019, 1020, 1021, 1022, 1023, 1024, 1025, 1026, 1027, 1028, 1029, 1030, 1031, 1032, 1033, 1034, 1035, 1036, 1037, 1038, 1039, 1040, 1041, 1042, 1043, 1044, 1045, 1046, 1047, 1048, 1049, 1050, 1051, 1052, 1053, 1054, 1055, 1056, 1057, 1058, 1059, 1060, 1061, 1062, 1063, 1064, 1065, 1066, 1067, 1068, 1069, 1070, 1071, 1072, 1073, 1074, 1075, 1076, 1077, 1078, 1079, 1080, 1081, 1082, 1083, 1084, 1085, 1086, 1087, 1088, 1089, 1090, 1091, 1092, 1093, 1094, 1095, 1096, 1097, 1098, 1099, 1100, 1101, 1102, 1103, 1104, 1105, 1106, 1107, 1108, 1109, 1110, 1111, 1112, 1113, 1114, 1115, 1116, 1117, 1118, 1119, 1120, 1121, 1122, 1123, 1124, 1125, 1126, 1127, 1128, 1129, 1130, 1131, 1132, 1133, 1134, 1135, 1136, 1137, 1138, 1139, 1140, 1141, 1142, 1143, 1144, 1145, 1146, 1147, 1148, 1149, 1150, 1151, 1152, 1153, 1154, 1155, 1156, 1157, 1158, 1159, 1160, 1161, 1162, 1163, 1164, 1165, 1166, 1167, 1168, 1169, 1170, 1171, 1172, 1173, 1174, 1175, 1176, 1177, 1178, 1179, 1180, 1181, 1182, 1183, 1184, 1185, 1186, 1187, 1188, 1189, 1190, 1191, 1192, 1193, 1194, 1195, 1196, 1197, 1198, 1199, 1200, 1201, 1202, 1203, 1204, 1205, 1206, 1207, 1208, 1209, 1210, 1211, 1212, 1213, 1214, 1215, 1216, 1217, 1218, 1219, 1220, 1221, 1222, 1223, 1224, 1225, 1226, 1227, 1228, 1229, 1230, 1231, 1232, 1233, 1234, 1235, 1236, 1237, 1238, 1239, 1240, 1241, 1242, 1243, 1244, 1245, 1246, 1247, 1248, 1249, 1250, 1251, 1252, 1253, 1254, 1255, 1256, 1257, 1258, 1259, 1260, 1261, 1262, 1263, 1264, 1265, 1266, 1267, 1268, 1269, 1270, 1271, 1272, 1273, 1274, 1275, 1276, 1277, 1278, 1279, 1280, 1281, 1282, 1283, 1284, 1285, 1286, 1287, 1288, 1289, 1290, 1291, 1292, 1293, 1294, 1295, 1296, 1297, 1298, 1299, 1300, 1301, 1302, 1303, 1304, 1305, 1306, 1307, 1308, 1309, 1310, 1311, 1312, 1313, 1314, 1315, 1316, 1317, 1318, 1319, 1320, 1321, 1322, 1323, 1324, 1325, 1326, 1327, 1328, 1329, 1330, 1331, 1332, 1333, 1334, 1335, 1336, 1337, 1338, 1339, 1340, 1341, 1342, 1343, 1344, 1345, 1346, 1347, 1348, 1349, 1350, 1351, 1352, 1353, 1354, 1355, 1356, 1357, 1358, 1359, 1360, 1361, 1362, 1363, 1364, 1365, 1366, 1367, 1368, 1369, 1370, 1371, 1372, 1373, 1374, 1375, 1376, 1377, 1378, 1379, 1380, 1381, 1382, 1383, 1384, 1385, 1386, 1387, 1388, 1389, 1390, 1391, 1392, 1393, 1394, 1395, 1396, 1397, 1398, 1399, 1400, 1401, 1402, 1403, 1404, 1405, 1406, 1407, 1408, 1409, 1410, 1411, 1412, 1413, 1414, 1415, 1416, 1417, 1418, 1419, 1420, 1421, 1422, 1423, 1424, 1425, 1426, 1427, 1428, 1429, 1430, 1431, 1432, 1433, 1434, 1435, 1436, 1437, 1438, 1439, 1440, 1441, 1442, 1443, 1444, 1445, 1446, 1447, 1448, 1449, 1450, 1451, 1452, 1453, 1454, 1455, 1456, 1457, 1458, 1459, 1460, 1461, 1462, 1463, 1464, 1465, 1466, 1467, 1468, 1469, 1470, 1471, 1472, 1473, 1474, 1475, 1476, 1477, 1478, 1479, 1480, 1481, 1482, 1483, 1484, 1485, 1486, 1487, 1488, 1489, 1490, 1491, 1492, 1493, 1494, 1495, 1496, 1497, 1498, 1499, 1500, 1501, 1502, 1503, 1504, 1505, 1506, 1507, 1508, 1509, 1510, 1511, 1512, 1513, 1514, 1515, 1516, 1517, 1518, 1519, 1520, 1521, 1522, 1523, 1524, 1525, 1526, 1527, 1528, 1529, 1530, 1531, 1532, 1533, 1535, 1536, 1537, 1538, 1539, 1540, 1541, 1542, 1543, 1544, 1545, 1546, 1547, 1548, 1549, 1550, 1551, 1552, 1553, 1554, 1555, 1556, 1557, 1558, 1559, 1560, 1561, 1562, 1563, 1564, 1565, 1566, 1567, 1568, 1569, 1570, 1571, 1572, 1573, 1574, 1575, 1576, 1577, 1578, 1579, 1580, 1581, 1582, 1583, 1584, 1585, 1586, 1587, 1588, 1589, 1590, 1591, 1592, 1593, 1594, 1595, 1596, 1597, 1598, 1599, 1600, 1601, 1602, 1603, 1604, 1605, 1606, 1607, 1608, 1609, 1610, 1611, 1612, 1613, 1614, 1615, 1616, 1617, 1618, 1619, 1620, 1621, 1622, 1623, 1624, 1625, 1626, 1627, 1628, 1629, 1630, 1631, 1632, 1633, 1634, 1635, 1636, 1637, 1638, 1639, 1640, 1641, 1642, 1643, 1644, 1645, 1646, 1647, 1648, 1649, 1650, 1651, 1652, 1653, 1654, 1655, 1656, 1657, 1658, 1659, 1660, 1661, 1662, 1663, 1664, 1665, 1666, 1667, 1668, 1669, 1670, 1671, 1672, 1673, 1674, 1675, 1676, 1677, 1678, 1679, 1680, 1681, 1682, 1683, 1684, 1685, 1686, 1687, 1688, 1689, 1690, 1691, 1692, 1693, 1694, 1695, 1696, 1697, 1698, 1699, 1700, 1701, 1702, 1703, 1704, 1705, 1706, 1707, 1708, 1709, 1710, 1711, 1712, 1713, 1714, 1715, 1716, 1717, 1718, 1719, 1720, 1721, 1722, 1723, 1724, 1725, 1726, 1727, 1728, 1729, 1730, 1731, 1732, 1733, 1734, 1735, 1736, 1737, 1738, 1739, 1740, 1741, 1742, 1743, 1744, 1745, 1746, 1747, 1748, 1749, 1750, 1751, 1752, 1753, 1754, 1755, 1756, 1757, 1758, 1759, 1760, 1761, 1762, 1763, 1764, 1765, 1766, 1767, 1768, 1769, 1770, 1771, 1772, 1773, 1774, 1775, 1776, 1777, 1778, 1779, 1780, 1781, 1782, 1783, 1784, 1785, 1786, 1787, 1788, 1789, 1790, 1791, 1792, 1793, 1794, 1795, 1796, 1797, 1798, 1799, 1800, 1801, 1802, 1803, 1804, 1805, 1806, 1807, 1808, 1809, 1810, 1811, 1812, 1813, 1814, 1815, 1816, 1817, 1818, 1819, 1820, 1821, 1822, 1823, 1824, 1825, 1826, 1827, 1828, 1829, 1830, 1831, 1832, 1833, 1834, 1835, 1836, 1837, 1838, 1839, 1840, 1841, 1842, 1843, 1844, 1845, 1846, 1847, 1848, 1849, 1850, 1851, 1852, 1853, 1854, 1855, 1856, 1857, 1858, 1859, 1860, 1861, 1862, 1863, 1864, 1865, 1866, 1867, 1868, 1869, 1870, 1871, 1873, 1874, 1875, 1876, 1879, 1880, 1881, 1882, 1883, 1884, 1885, 1886, 1887, 1888, 1889, 1890, 1891, 1892, 1893, 1894, 1895, 1897, 1898, 1899, 1900, 1901, 1902, 1903, 1904, 1905, 1906, 1907, 1908, 1909, 1910, 1911, 1912, 1913, 1914, 1915, 1916, 1917, 1918, 1919, 1920, 1921, 1922, 1923, 1924, 1925, 1926, 1927, 1928, 1929, 1930, 1931, 1932, 1933, 1934, 1935, 1936, 1937, 1938, 1939, 1940, 1941, 1942, 1943, 1944, 1945, 1946, 1947, 1948, 1949, 1950, 1951, 1952, 1953, 1954, 1955, 1956, 1957, 1958, 1959, 1960, 1961, 1962, 1963, 1964, 1965, 1966, 1967, 1968, 1969, 1970, 1971, 1972, 1973, 1974, 1975, 1976, 1977, 1978, 1979, 1980, 1981, 1982, 1983, 1984, 1985, 1986, 1987, 1988, 1989, 1990, 1991, 1992, 1993, 1994, 1995, 1996, 1997, 1998, 1999, 2000, 2001, 2002, 2003, 2004, 2005, 2006, 2007, 2008, 2009, 2010, 2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020, 2021, 2022,
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
	86, 14268, 14269, 14316, 15739, 15777, 15778, 15779, 15780, 15781, 15782, 15783, 15784, 15785, 15786, 15787, 15788, 15789, 15790, 15791, 15792, 15793, 15794, 15795, 15796, 15797, 15798, 15799, 15800, 15801, 15802, 15803, 15804, 15805, 15806, 15807, 15808, 15809, 15810, 15811, 15812, 15813, 15814, 15815, 15816, 15817, 15818, 15819, 15820, 15821, 15822, 15823, 15824, 15825, 15826, 15827, 15828, 15829, 15830, 15831, 15832, 15833, 15834, 15835, 15836, 15837, 15838, 15839, 15840, 15841, 15842, 15843, 15844, 15845, 15846, 15847, 15848, 15849, 15850, 15851, 15852, 15853, 15854, 15855, 15856, 15857, 15858, 15859, 15860, 15861, 15862, 15863, 15864, 15865, 15866, 15867, 15868, 15869, 15870, 15871, 15872, 15873, 15874, 15875, 15876, 15877, 15878, 15879, 15880, 15881, 15882, 15883, 15884, 15885, 15886, 15887, 15888, 15889, 15890, 15891, 15892, 15893, 15894, 15895, 15896, 15897, 15898, 15899, 15900, 15901, 15902, 15903, 15904, 15905, 15906, 15907, 15908, 15909, 15910, 15911, 15912, 15913, 15914, 15915, 15916, 15917, 15918, 15919, 15920, 15921, 15922, 15923, 15924, 15925, 15926, 15927, 15928, 15929, 15930, 15931, 15932, 15933, 15934, 15935, 15936, 15937, 15938, 15939, 15940, 15941, 15942, 15943, 15944, 15945, 15946, 15947, 15948, 15949, 15950, 15951, 15952, 15953, 15954, 15955, 15956, 15957, 15958, 15959, 15960, 15961, 15962, 15963, 15964, 15965, 15966, 15967, 15968, 15969, 15970, 15971, 15972, 15973, 15974, 15975, 15976, 15977, 15978, 15979, 15980, 15981, 15982, 15983, 15984, 15985, 15986, 15987, 15988, 15989, 15990, 15991, 15992, 15993, 15994, 15995, 15996, 15997, 15998, 15999, 16000, 16001, 16002, 16003, 16004, 16005, 16006, 16007, 16008, 16009, 16010, 16011, 16012, 16013, 16014, 16015, 16016, 16017, 16018, 16019, 16020, 16021, 16022, 16023, 16024, 16025, 16026, 16027, 16028, 16029, 16030, 16031, 16032, 16033, 16034, 16035, 16036, 16037, 16038, 16039, 16040, 16041, 16042, 16043, 16044, 16045, 16046, 16047, 16048, 16049, 16050, 16051, 16052, 16053, 16054, 16055, 16056, 16057, 16058, 16059, 16060, 16061, 16062, 16063, 16064, 16065, 16066, 16067, 16068, 16069, 16070, 16071, 16072, 16073, 16074, 16075, 16076, 16077, 16078, 16079, 16080, 16081, 16082, 16083, 16084, 16085, 16086, 16087, 16088, 16089, 16090, 16091, 16092, 16093, 16094, 16095, 16096, 16097, 16098, 16099, 16100, 16101, 16102, 16103, 16104, 16105, 16106, 16107, 16108, 16109, 16110, 16111, 16112, 16113, 16114, 16115, 16116, 16117, 16118, 16119, 16120, 16121, 16122, 16123, 16124, 16125, 16126, 16127, 16128, 16129, 16130, 16131, 16132, 16133, 16134, 16135, 16136, 16137, 16138, 16139, 16140, 16141, 16142, 16143, 16144, 16145, 16146, 16147, 16148, 16149, 16150, 16151, 16152, 16153, 16154, 16155, 16156, 16157, 16158, 16159, 16160, 16161, 16162, 16163, 16164, 16165, 16166, 16167, 16168, 16169, 16170, 16171, 16172, 16173, 16174, 16175, 16176, 16177, 16178, 16179, 16180, 16181, 16182, 16183, 16184, 16185, 16186, 16187, 16188, 16189, 16190, 16191, 16192, 16193, 16194, 16195, 16196, 16197, 16198, 16199, 16200, 16201, 16202, 16203, 16204, 16205, 16206, 16207, 16208, 16209, 16210, 16211, 16212, 16213, 16214, 16215, 16216, 16217, 16218, 16219, 16220, 16221, 16222, 16223, 16224, 16225, 16226, 16227, 16228, 16229, 16230, 16231, 16232, 16233, 16234, 16235, 16236, 16237, 16238, 16239, 16240, 16241, 16242, 16243, 16244, 16245, 16246, 16247, 16248, 16249, 16250, 16251, 16252, 16253, 16254, 16255, 16256, 16257, 16258, 16259, 16260, 16261, 16262, 16263, 16264, 16265, 16266, 16267, 16268, 16269, 16270, 16271, 16272, 16273, 16274, 16275, 16276, 16277, 16278, 16279, 16280, 16281, 16282, 16283, 16284, 16285, 16286, 16287, 16288, 16289, 16290, 16291, 16292, 16293, 16294, 16295, 16296, 16297, 16298, 16299, 16300, 16301, 16302, 16303, 16304, 16305, 16306, 16307, 16308, 16309, 16310, 16311, 16312, 16313, 16314, 16315, 16316, 16317, 16318, 16319, 16320, 16321, 16322, 16323, 16324, 16325, 16326, 16327, 16328, 16329, 16330, 16331, 16332, 16333, 16334, 16335, 16336, 16337, 16338, 16339, 16340, 16341, 16342, 16343, 16344, 16345, 16346, 16347, 16348, 16349, 16350, 16351, 16352, 16353, 16354, 16355, 16356, 16357, 16358, 16359, 16360, 16361, 16362, 16363, 16364, 16365, 16366, 16367, 16368, 16369, 16370, 16371, 16372, 16373, 16374, 16375, 16376, 16377, 16378, 16379, 16380, 16381, 16382, 16383, 16384, 16385, 16386, 16387, 16388, 16389, 16390, 16391, 16392, 16393, 16394, 16395, 16396, 16397, 16398, 16399, 16400, 16401, 16402, 16403, 16404, 16405, 16406, 16407, 16408, 16409, 16410, 16411, 16412, 16413, 16414, 16415, 16416, 16417, 16418, 16419, 16420, 16421, 16422, 16423, 16424, 16425, 16426, 16427, 16428, 16429, 16430, 16431, 16432, 16433, 16434, 16435, 16436, 16437, 16438, 16439, 16440, 16441, 16442, 16443, 16444, 16445, 16446, 16447, 16448, 16449, 16450, 16451, 16452, 16453, 16454, 16455, 16456, 16457, 16458, 16459, 16460, 16461, 16462, 16463, 16464, 16465, 16466, 16467, 16468, 16469, 16470, 16471, 16472, 16473, 16474, 16475, 16476, 16477, 16478, 16479, 16480, 16481, 16482, 16483, 16484, 16485, 16486, 16487, 16488, 16489, 16490, 16491, 16492, 16493, 16494, 16495, 16496, 16497, 16498, 16499, 16500, 16501, 16502, 16503, 16504, 16505, 16506, 16507, 16508, 16509, 16510, 16511, 16512, 16513, 16514, 16515, 16516, 16517, 16518, 16519, 16520, 16521, 16522, 16523, 16524, 16525, 16526, 16527, 16528, 16529, 16530, 16531, 16532, 16533, 16534, 16535, 16536, 16537, 16538, 16539, 16540, 16541, 16542, 16543, 16544, 16545, 16546, 16547, 16548, 16549, 16550, 16551, 16552, 16553, 16554, 16555, 16556, 16557, 16558, 16559, 16560, 16561, 16562, 16563, 16564, 16565, 16566, 16567, 16568, 16569, 16570, 16571, 16572, 16573, 16574, 16575, 16576, 16577, 16578, 16579, 16580, 16581, 16582, 16583, 16584, 16585, 16586, 16587, 16588, 16589, 16590, 16591, 16592, 16593, 16594, 16595, 16597, 16598, 16599, 16600, 16601, 16602, 16603, 16604, 16605, 16606, 16607, 16608, 16609, 16610, 16611, 16612, 16613, 16614, 16615, 16616, 16617, 16618, 16619, 16620, 16621, 16622, 16623, 16624, 16625, 16626, 16627, 16628, 16629, 16630, 16631, 16632, 16633, 16634, 16635, 16636, 16637, 16638, 16639, 16640, 16641, 16642, 16643, 16644, 16645, 16646, 16647, 16648, 16649, 16650, 16651, 16652, 16653, 16654, 16655, 16656, 16657, 16658, 16659, 16660, 16661, 16662, 16663, 16664, 16665, 16666, 16667, 16668, 16669, 16670, 16671, 16672, 16673, 16674, 16675, 16676, 16677, 16678, 16679, 16680, 16681, 16682, 16683, 16684, 16685, 16686, 16687, 16688, 16689, 16690, 16691, 16692, 16693, 16694, 16695, 16696, 16697, 16698, 16699, 16700, 16701, 16702, 16703, 16704, 16705, 16706, 16707, 16708, 16709, 16710, 16711, 16712, 16713, 16714, 16715, 16716, 16717, 16718, 16719, 16720, 16721, 16722, 16723, 16724, 16725, 16726, 16727, 16728, 16729, 16730, 16731, 16732, 16733, 16734, 16735, 16736, 16737, 16738, 16739, 16740, 16741, 16742, 16743, 16744, 16745, 16746, 16747, 16748, 16749, 16750, 16751, 16752, 16753, 16754, 16755, 16756, 16757, 16758, 16759, 16760, 16761, 16762, 16763, 16764, 16765, 16766, 16767, 16768, 16769, 16770, 16771, 16772, 16773, 16774, 16775, 16776, 16777, 16778, 16779, 16780, 16781, 16782, 16783, 16784, 16785, 16786, 16787, 16788, 16789, 16790, 16791, 16792, 16793, 16794, 16795, 16796, 16797, 16798, 16799, 16800, 16801, 16802, 16803, 16804, 16805, 16806, 16807, 16808, 16809, 16810, 16811, 16812, 16813, 16814, 16815, 16816, 16817, 16818, 16819, 16820, 16821, 16822, 16823, 16824, 16825, 16826, 16827, 16828, 16829, 16830, 16831, 16832, 16833, 16834, 16835, 16836, 16837, 16838, 16839, 16840, 16841, 16842, 16843, 16844, 16845, 16846, 16847, 16848, 16849, 16850, 16851, 16852, 16853, 16854, 16855, 16856, 16857, 16858, 16859, 16860, 16861, 16862, 16863, 16864, 16865, 16866, 16867, 16868, 16869, 16870, 16871, 16872, 16873, 16874, 16875, 16876, 16877, 16878, 16879, 16880, 16881, 16882, 16883, 16884, 16885, 16886, 16887, 16888, 16889, 16890, 16891, 16892, 16893, 16894, 16895, 16896, 16897, 16898, 16899, 16900, 16901, 16902, 16903, 16904, 16905, 16906, 16907, 16908, 16909, 16910, 16911, 16912, 16913, 16914, 16915, 16916, 16917, 16918, 16919, 16920, 16921, 16922, 16923, 16924, 16925, 16926, 16927, 16928, 16929, 16930, 16931, 16932, 16933, 16935, 16936, 16937, 16938, 16941, 16942, 16943, 16944, 16945, 16946, 16947, 16948, 16949, 16950, 16951, 16952, 16953, 16954, 16955, 16956, 16957, 16959, 16960, 16961, 16962, 16963, 16964, 16965, 16966, 16967, 16968, 16969, 16970, 16971, 16972, 16973, 16974, 16975, 16976, 16977, 16978, 16979, 16980, 16981, 16982, 16983, 16984, 16985, 16986, 16987, 16988, 16989, 16990, 16991, 16992, 16993, 16994, 16995, 16996, 16997, 16998, 16999, 17000, 17001, 17002, 17003, 17004, 17005, 17006, 17007, 17008, 17009, 17010, 17011, 17012, 17013, 17014, 17015, 17016, 17017, 17018, 17019, 17020, 17021, 17022, 17023, 17024, 17025, 17026, 17027, 17028, 17029, 17030, 17031, 17032, 17033, 17034, 17035, 17036, 17037, 17038, 17039, 17040, 17041, 17042, 17043, 17044, 17045, 17046, 17047, 17048, 17049, 17050, 17051, 17052, 17053, 17054, 17055, 17056, 17057, 17058, 17059, 17060, 17061, 17062, 17063, 17064, 17065, 17066, 17067, 17068, 17069, 17070, 17071, 17072, 17073, 17074, 17075, 17076, 17077, 17078, 17079, 17080, 17081, 17082, 17083, 17084, 17085, 17086, 17087, 17088, 17089, 17090, 17091, 17092, 17093, 17094, 17095, 17096, 17097, 17098, 17099, 17100, 17101, 17102, 17103, 17104, 17105, 17106, 17107, 17108, 17109, 17110, 17111, 17112, 17113, 17114, 17115, 17116, 17117, 17118, 17119, 17120, 17121, 17122, 17123, 17124, 17125, 17126, 17127, 17128, 17129, 17130, 17131, 17132, 17133, 17134, 17135, 17136, 17137, 17138, 17139, 17140, 17141, 17142, 17143, 17144, 17145, 17146, 17147, 17148, 17149, 17150, 17151, 17152, 17153, 17154, 17155, 17156, 17157, 17158, 17159, 17160, 17161, 17162, 17163, 17164, 17165, 17166, 17167, 17168, 17169, 17170, 17171, 17172, 17173, 17174, 17175, 17177, 17178, 17179, 17180, 17181, 17182, 17183, 17184, 17185, 17186, 17187, 17188, 17189, 17190, 17191, 17192, 17215, 17216, 17217, 17218, 17219, 17220, 17221, 17222, 17223, 17224, 17225, 17226, 17227,
}
func main() {
	// Define the MediaID here. Irs required only when executing "RunMediaRelations()"
	mediaIDs := []int{}
	p := NewPipeLineRunner(
		// Define the environment
		EnvironmentProd,
		// Define the environment specific vertical ID
		// TyreVerticalIDProd,
		BatteryVerticalIDProd,
		BatteryVariationIDsProd,
		
		// Define the environment specific variation ID
		// TyreVariationIDsProd,
		mediaIDs,
	)
	// Uncomment only the required function calls and execute the script. Prefer to do one at a time and verify.
	// Attaching variation to op assets
	// RunOpsAsset(p)
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


