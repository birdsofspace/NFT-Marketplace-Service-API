package main

import (
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

func main() {

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
			if strings.Contains(rpc, "https") {
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
		return fastestRpc
	}

	db, _ := sql.Open("sqlite3", "./nft.db")
	defer db.Close()
	null_address := common.HexToAddress("0x0000000000000000000000000000000000000000")
	conn, err := ethclient.Dial(getRPC(137))
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	address := common.HexToAddress("0xbd71d373556867dbb589f2c7cc464882fafd52be")
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
		_, _ = stmt.Exec(id, ip)
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
		_, _ = stmt.Exec(id, ip)
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
		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS nft_top (id INTEGER, ip_address TEXT)`)
		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS nft (id INTEGER, owner TEXT, token_uri TEXT)`)
		tx, _ := db.Begin()
		stmt, _ := tx.Prepare("INSERT OR REPLACE INTO nft (id, owner, token_uri) VALUES (?,?,?)")
		i := 1

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

	r := mux.NewRouter()
	r.HandleFunc("/nfts", updateInBackground).Methods("PATCH")
	r.HandleFunc("/nfts", getTotalNFTs).Methods("GET")
	r.HandleFunc("/nfts/owner/{owner}", searchNFTByOwner).Methods("GET")
	r.HandleFunc("/nfts/{id}/owner", getOwnerByNFTID).Methods("GET")
	r.HandleFunc("/nfts/{id}/like", getLikesByNFTID).Methods("GET")
	r.HandleFunc("/nfts/{id}/like", likeNFTByIP).Methods("POST")
	r.HandleFunc("/nfts/{id}/dislike", dislikeNFTByIP).Methods("DELETE")
	http.ListenAndServe(":8000", r)
}
