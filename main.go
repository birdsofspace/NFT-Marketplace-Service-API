package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"database/sql"

	_ "github.com/mattn/go-sqlite3"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gorilla/mux"
	"github.com/metachris/eth-go-bindings/erc721"
)

type Chain struct {
	Name           string   `json:"name"`
	Chain          string   `json:"chain"`
	ChainId        string   `json:"chainId"`
	Network        string   `json:"network"`
	RPC            []string `json:"rpc"`
	Faucets        []string `json:"faucets"`
	InfoURL        string   `json:"infoURL"`
	ShortName      string   `json:"shortName"`
	ChainName      string   `json:"chainName"`
	NativeCurrency struct {
		Name     string `json:"name"`
		Symbol   string `json:"symbol"`
		Decimals int    `json:"decimals"`
	} `json:"nativeCurrency"`
}

type NFT struct {
	ID       int    `json:"id"`
	Owner    string `json:"owner"`
	TokenURI string `json:"token_uri"`
}

type StdRes struct {
	Status  string      `json:"status"`
	Data    interface{} `json:"data"`
	Message string      `json:"msg"`
}

type ResTotalNFTs struct {
	TotalNFTs int `json:"total_nfts"`
}

type ResOwnerNFT struct {
	Owner string `json:"owner"`
}

type LogSold struct {
	ItemId *big.Int       `json:"item_id"`
	Owner  common.Address `json:"owner"`
}

type LogSoldWithBlock struct {
	ItemId   *big.Int       `json:"item_id"`
	Owner    common.Address `json:"owner"`
	BlockNum *big.Int       `json:"block"`
}

type LogNewItem struct {
	ItemId      *big.Int
	NFTContract common.Address
	TokenId     *big.Int
	Seller      common.Address
	Owner       common.Address
	Price       *big.Int
	Sold        bool
}

func main() {
	var port string = "1370"
	var chain_selected int = 137
	var STATUS []string = []string{"failed", "success"}

	var getRPC = func(chid int) string {
		chainID := strconv.Itoa(chid)
		url := fmt.Sprintf("https://raw.githubusercontent.com/ethereum-lists/chains/master/_data/chains/eip155-%s.json", chainID)

		resp, _ := http.Get(url)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var chain Chain
		_ = json.Unmarshal(body, &chain)
		var fastestRpc string
		var fastestTime time.Duration
		fastestRpc = chain.RPC[0]
		for _, rpc := range chain.RPC {
			if strings.Contains(rpc, "wss") {
				start := time.Now()

				testCon, err := ethclient.Dial(rpc)
				if err != nil {
					continue
				}
				testCon.Close()
				defer resp.Body.Close()
				_, _ = io.ReadAll(resp.Body)
				elapsed := time.Since(start)
				if fastestTime == 0 || elapsed < fastestTime {
					fastestTime = elapsed
					fastestRpc = rpc
				}
			}

		}
		testCon, err := ethclient.Dial(fastestRpc)
		if err != nil {
			fastestRpc = chain.RPC[0]
		}
		testCon.Close()
		println(fastestRpc)
		fastestRpc = chain.RPC[0]
		return fastestRpc
	}

	var getABI = func(chid int) abi.ABI {
		id := strconv.Itoa(chid)
		t := time.Now().Unix()
		resp, _ := http.Get("https://raw.githubusercontent.com/birdsofspace/global-config/main/" + id + "/MARKETPLACE/ABI.json?time=" + strconv.FormatInt(t, 10))
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		contractAbi, _ := abi.JSON(strings.NewReader(string(body)))
		return contractAbi
	}

	var getMarketAddress = func(chid int) string {
		id := strconv.Itoa(chid)
		t := time.Now().Unix()
		resp, _ := http.Get("https://raw.githubusercontent.com/birdsofspace/global-config/main/" + id + "/MARKETPLACE/CONTRACT_ADDRESS?time=" + strconv.FormatInt(t, 10))
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return strings.TrimSpace(string(body))
	}

	var getNFTAddress = func(chid int) string {
		id := strconv.Itoa(chid)
		t := time.Now().Unix()
		resp, _ := http.Get("https://raw.githubusercontent.com/birdsofspace/global-config/main/" + id + "/ERC-721/CONTRACT_ADDRESS?time=" + strconv.FormatInt(t, 10))
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return strings.TrimSpace(string(body))
	}

	db, _ := sql.Open("sqlite3", "./"+strconv.Itoa(chain_selected)+"nft.db")
	defer db.Close()
	null_address := common.HexToAddress("0x0000000000000000000000000000000000000000")
	conn, err := ethclient.Dial(getRPC(chain_selected))
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	address := common.HexToAddress(getNFTAddress(chain_selected))
	token, err := erc721.NewErc721(address, conn)
	if err != nil {
		log.Fatalf("Failed to instantiate a Token contract: %v", err)
	}

	name, err := token.Name(nil)
	if err != nil {
		log.Fatalf("Failed to retrieve token name: %v", err)
	}
	fmt.Println("Token name:", name)
	balance, _ := token.BalanceOf(nil, address)
	println(balance.Int64())

	var getTotalNFTs = func(w http.ResponseWriter, r *http.Request) {
		var t int = 0
		_ = db.QueryRow("SELECT COUNT(*) FROM nft").Scan(&t)
		rsp := StdRes{
			Status: STATUS[1],
			Data: ResTotalNFTs{
				TotalNFTs: t,
			},
			Message: "Total NFTs retrieved successfully",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rsp)
	}
	var searchNFTByOwner = func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		own := params["owner"]
		rows, _ := db.Query("SELECT * FROM nft WHERE owner =?", own)
		defer rows.Close()
		var nfts []NFT
		for rows.Next() {
			var nft NFT
			_ = rows.Scan(&nft.ID, &nft.Owner, &nft.TokenURI)
			nfts = append(nfts, nft)
		}
		rsp := StdRes{
			Status:  STATUS[1],
			Data:    nfts,
			Message: "NFTs found for owner successfully",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rsp)
	}

	var getOwnerByNFTID = func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		id, _ := strconv.Atoi(params["id"])
		var own string
		_ = db.QueryRow("SELECT owner FROM nft WHERE id =?", id).Scan(&own)
		rsp := StdRes{
			Status: STATUS[1],
			Data: ResOwnerNFT{
				Owner: own,
			},
			Message: "NFTs found for owner successfully",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rsp)
	}

	var likeNFTByIP = func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		id, _ := strconv.Atoi(params["id"])
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		tx, _ := db.Begin()
		stmt, _ := tx.Prepare("INSERT OR REPLACE INTO nft_top (id, ip_address) VALUES (?,?)")
		_, _ = stmt.Exec(id, base64.StdEncoding.EncodeToString([]byte(ip)))
		_ = tx.Commit()
		var count int
		_ = db.QueryRow("SELECT COUNT(*) FROM nft_top WHERE id =?", id).Scan(&count)

		rsp := StdRes{
			Status: STATUS[1],
			Data: struct {
				Likes int `json:"likes"`
			}{Likes: count},
			Message: "NFT like successfully recorded from IP address",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rsp)
	}

	var dislikeNFTByIP = func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		id, _ := strconv.Atoi(params["id"])
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		tx, _ := db.Begin()
		stmt, _ := tx.Prepare("DELETE FROM nft_top WHERE id =? AND ip_address =?")
		_, _ = stmt.Exec(id, base64.StdEncoding.EncodeToString([]byte(ip)))
		_ = tx.Commit()
		var count int
		_ = db.QueryRow("SELECT COUNT(*) FROM nft_top WHERE id =?", id).Scan(&count)

		rsp := StdRes{
			Status: STATUS[1],
			Data: struct {
				Likes int `json:"likes"`
			}{Likes: count},
			Message: "NFT dislike successfully recorded from IP address",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rsp)
	}

	var getLikesByNFTID = func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		id, _ := strconv.Atoi(params["id"])
		var count int
		_ = db.QueryRow("SELECT COUNT(*) FROM nft_top WHERE id =?", id).Scan(&count)

		rsp := StdRes{
			Status: STATUS[1],
			Data: struct {
				Likes int `json:"likes"`
			}{Likes: count},
			Message: "Total NFT likes retrieved successfully",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rsp)
	}

	var updateDBNFT = func() {
		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS nft_notification (id INTEGER, owner TEXT, block INTEGER, UNIQUE (id, owner, block) ON CONFLICT REPLACE);`)
		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS nft_top (id INTEGER, ip_address TEXT, UNIQUE (id, ip_address) ON CONFLICT REPLACE);`)
		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS nft (id INTEGER, owner TEXT, token_uri TEXT, UNIQUE (id, owner) ON CONFLICT REPLACE);`)
		tx, _ := db.Begin()
		stmt, _ := tx.Prepare("INSERT OR REPLACE INTO nft (id, owner, token_uri) VALUES (?,?,?)")
		i := 0

		for {
			bi := big.NewInt(int64(i))
			own, _ := token.OwnerOf(nil, bi)
			if own == null_address {
				break
			}
			turi, _ := token.TokenURI(nil, bi)
			_, _ = stmt.Exec(strconv.Itoa(i), own.String(), turi)
			i++
		}
		_ = tx.Commit()

	}

	qL := ethereum.FilterQuery{
		Addresses: []common.Address{common.HexToAddress(getMarketAddress(chain_selected))},
	}

	sB, _ := conn.FilterLogs(context.Background(), qL)

	go func() {
		for {
			for _, vLog := range sB {
				switch vLog.Topics[0].Hex() {
				case "0x045dfa01dcba2b36aba1d3dc4a874f4b0c5d2fbeb8d2c4b34a7d88c8d8f929d1":
					var LogSoldf LogSold
					LogSoldf.ItemId = (vLog.Topics[1].Big())
					LogSoldf.Owner = common.HexToAddress(vLog.Topics[2].Hex())

					tx, err := db.Begin()
					if err != nil {
						fmt.Println("Failed to begin transaction:", err)
						continue
					}

					stmt, err := tx.Prepare("INSERT OR REPLACE INTO nft_notification (id, owner, block) VALUES (?,?,?)")
					if err != nil {
						fmt.Println("Failed to prepare statement:", err)
						continue
					}

					_, err = stmt.Exec(LogSoldf.ItemId.Int64(), base64.StdEncoding.EncodeToString([]byte(LogSoldf.Owner.String())), vLog.BlockNumber)
					if err != nil {
						fmt.Println("Failed to execute statement:", err)
						continue
					}

					if err := tx.Commit(); err != nil {
						fmt.Println("Failed to commit transaction:", err)
					}
				}
			}
		}
	}()

	var getNotificationByOwner = func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		own := params["owner"]
		rows, _ := db.Query("SELECT * FROM nft_notification WHERE owner =?", own)
		defer rows.Close()
		var results []LogSoldWithBlock
		for rows.Next() {
			var result LogSoldWithBlock
			_ = rows.Scan(&result.ItemId, &result.Owner, &result.BlockNum)
			results = append(results, result)
		}
		rsp := StdRes{
			Status:  STATUS[1],
			Data:    results,
			Message: "Successfully fetched notifications!",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rsp)
	}

	var lastSold = func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		var block int64
		fmt.Sscan(params["block"], &block)
		blockNum, _ := conn.HeaderByNumber(context.Background(), nil)
		if block == 0 {
			block = blockNum.Number.Int64()
		}

		querySold := ethereum.FilterQuery{
			FromBlock: big.NewInt(block - 1000),
			ToBlock:   big.NewInt(block),
			Addresses: []common.Address{
				common.HexToAddress(getMarketAddress(chain_selected)),
			},
		}

		logs, _ := conn.FilterLogs(context.Background(), querySold)
		var data []LogSold
		for _, vLog := range logs {
			switch vLog.Topics[0].Hex() {
			case "0x045dfa01dcba2b36aba1d3dc4a874f4b0c5d2fbeb8d2c4b34a7d88c8d8f929d1":
				var LogSoldf LogSold
				LogSoldf.ItemId = (vLog.Topics[1].Big())
				LogSoldf.Owner = common.HexToAddress(vLog.Topics[2].Hex())
				data = append(data, LogSoldf)
			}
		}
		rsp := StdRes{
			Status:  STATUS[1],
			Data:    data,
			Message: "Log Sold Out retrieved successfully",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rsp)
	}

	var updateInBackground = func(w http.ResponseWriter, r *http.Request) {

		updateChannel := make(chan struct{})
		go func() {
			defer close(updateChannel)
			updateDBNFT()
		}()

		rsp := StdRes{
			Status:  STATUS[1],
			Data:    nil,
			Message: "NFTs updated in the background",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rsp)

	}

	defer getABI(chain_selected)

	r := mux.NewRouter()
	r.HandleFunc("/nfts", updateInBackground).Methods("PATCH")
	r.HandleFunc("/nfts", getTotalNFTs).Methods("GET")
	r.HandleFunc("/nfts/sold/{block}", lastSold).Methods("GET")
	r.HandleFunc("/nfts/owner/{owner}", searchNFTByOwner).Methods("GET")
	r.HandleFunc("/nfts/{id}/owner", getOwnerByNFTID).Methods("GET")
	r.HandleFunc("/nfts/{id}/like", getLikesByNFTID).Methods("GET")
	r.HandleFunc("/nfts/{id}/like", likeNFTByIP).Methods("POST")
	r.HandleFunc("/nfts/{id}/like", dislikeNFTByIP).Methods("DELETE")
	r.HandleFunc("/notifications/user/{owner}", getNotificationByOwner).Methods("GET")
	http.ListenAndServe(":"+port, r)
}
