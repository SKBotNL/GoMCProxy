// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/fatih/color"
)

type State int

const (
	StateHandshaking State = iota
	StateStatus
	StateLogin
	StatePlay
)

type ChatType byte

const (
	ChatTypeChat ChatType = iota
	ChatTypeSystem
	ChatTypeActionBar
)

type Proxy struct {
	state           State
	threshold       int
	sharedSecret    []byte
	serverPublicKey *rsa.PublicKey
	serverDecrypt   cipher.Stream
	serverEncrypt   cipher.Stream
	serverReader    *cipher.StreamReader
	serverWriter    *cipher.StreamWriter
	shouldExit      bool
	exited          chan struct{}
	forwardAddr     string
	accessToken     string
	uuid            string
	isHypixel       bool
	bedwarsType     *BedwarsType
}

var hypixel *Hypixel

func main() {
	listenHost := flag.String("listenhost", "127.0.0.1", "The host to listen on")
	listenPort := flag.String("listenport", "25565", "The port to listen on")

	forwardHost := flag.String("forwardhost", "mc.hypixel.net", "The host to forward to")
	forwardPort := flag.String("forwardport", "25565", "The port to forward to")

	accessToken := flag.String("accesstoken", "", "Mojang Access Token. See https://kqzz.github.io/mc-bearer-token/")

	uuid := flag.String("uuid", "", "Your Minecraft account's UUID")

	hak := flag.String("hypixel-api-key", "", "Hypixel API Key")

	flag.Parse()

	listenAddr := *listenHost + ":" + *listenPort
	forwardAddr := *forwardHost + ":" + *forwardPort

	if *accessToken == "" {
		color.Red("No Mojang Access Token has been provided")
		return
	}

	uuidRegex := regexp.MustCompile(`[0-9a-fA-F]{8}\b-[0-9a-fA-F]{4}\b-[0-9a-fA-F]{4}\b-[0-9a-fA-F]{4}\b-[0-9a-fA-F]{12}`)
	if *uuid == "" {
		color.Red("No UUID has been provided")
		return
	}
	if !uuidRegex.Match([]byte(*uuid)) {
		color.Red("An invalid UUID has been provided")
		return
	}

	if *hak == "" {
		color.Yellow("No Hypixel API Key has been provided, Hypixel API features will be disabled")
	} else {
		hypixel = newHypixel(*hak)

		valid, err := hypixel.testKey()
		if err != nil {
			color.Red("An error occurred while testing the Hypixel API Key: ", err)
			return
		}
		if !valid {
			color.Red("Invalid Hypixel API Key")
			return
		}
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Panicf("Failed to listen on %s: %v", listenAddr, err)
	}
	defer ln.Close()
	log.Printf("Proxy listening on %s, forwarding to %s", listenAddr, forwardAddr)

	for {
		clientConn, err := ln.Accept()
		if err != nil {
			log.Panic(err)
			continue
		}
		go handleClient(clientConn, forwardAddr, *accessToken, *uuid)
	}
}

func handleClient(clientConn net.Conn, forwardAddr string, accessToken string, uuid string) {
	defer clientConn.Close()

	serverConn, err := net.Dial("tcp", forwardAddr)
	if err != nil {
		log.Panic(err)
		return
	}

	proxy := Proxy{
		state:           StateHandshaking,
		threshold:       -1,
		sharedSecret:    nil,
		serverPublicKey: nil,
		serverDecrypt:   nil,
		serverEncrypt:   nil,
		serverReader:    nil,
		serverWriter:    nil,
		shouldExit:      false,
		exited:          make(chan struct{}),
		forwardAddr:     forwardAddr,
		accessToken:     accessToken,
		uuid:            uuid,
		isHypixel:       false,
		bedwarsType:     nil,
	}
	go proxy.proxyTraffic(clientConn, serverConn, true)
	go proxy.proxyTraffic(serverConn, clientConn, false)

	<-proxy.exited
	serverConn.(*net.TCPConn).CloseWrite()
	serverConn.Close()

	log.Println("Cleared proxy state and closed the server connection")
}

func (p *Proxy) proxyTraffic(src net.Conn, dst net.Conn, clientToServer bool) {
	for {
		var r io.Reader = src
		if p.serverReader != nil && !clientToServer {
			r = p.serverReader
		}

		packetLength, packetData, err := p.readPacket(r)
		if err != nil {
			if p.errorChecker(err) {
				return
			}
		}
		if packetLength == 0 {
			log.Println("Packet length is 0")
			continue
		}

		packetReader := bytes.NewReader(packetData)
		packetID, _, err := readVarInt(packetReader)
		if err != nil {
			log.Panic(err)
		}

		// Handshake
		if p.state == StateHandshaking && packetID == 0 && clientToServer {
			// Protocol version
			protocolVersion, _, err := readVarInt(packetReader)
			if err != nil {
				log.Panic(err)
				return
			}
			if protocolVersion != 47 {
				log.Panic("This proxy only supports protocol version 47 (1.8.*)")
			}

			// Server address
			_, err = readPrefixedBytes(packetReader)
			if err != nil {
				log.Panic(err)
				return
			}

			// Server port
			_, err = io.CopyN(io.Discard, packetReader, 2)
			if err != nil {
				log.Panic(err)
				return
			}

			// Intent
			intent, _, err := readVarInt(packetReader)
			if err != nil {
				log.Panic(err)
				return
			}

			var reconstructedPacket bytes.Buffer
			var packetBody bytes.Buffer

			// Packet ID
			if err := writeVarInt(&packetBody, 0x00); err != nil {
				log.Panic(err)
			}

			// Protocol version
			if err := writeVarInt(&packetBody, protocolVersion); err != nil {
				log.Panic(err)
			}

			forwardAddrSplit := strings.Split(p.forwardAddr, ":")
			if len(forwardAddrSplit) != 2 {
				log.Panic(errors.New("Invalid forward addr"))
			}
			serverAddress := forwardAddrSplit[0]
			serverPortString := forwardAddrSplit[1]
			serverPortUint16, err := strconv.ParseUint(serverPortString, 10, 16)
			if err != nil {
				log.Panic(err)
			}
			serverPort := make([]byte, 2)
			binary.BigEndian.PutUint16(serverPort, uint16(serverPortUint16))

			// Server address length + Server address
			if err := writeVarInt(&packetBody, len(serverAddress)); err != nil {
				log.Panic(err)
			}
			packetBody.Write([]byte(serverAddress))

			// Server Port
			packetBody.Write(serverPort)

			// Intent
			if err := writeVarInt(&packetBody, intent); err != nil {
				log.Panic(err)
			}

			// Turn into a full packet
			if err := writeVarInt(&reconstructedPacket, packetBody.Len()); err != nil {
				log.Panic(err)
			}
			reconstructedPacket.Write(packetBody.Bytes())

			_, err = dst.Write(reconstructedPacket.Bytes())
			if err != nil {
				if p.errorChecker(err) {
					return
				}
			}

			switch intent {
			case 1:
				p.state = StateStatus
				log.Println("Switched to the Status state")
			case 2:
				p.state = StateLogin
				log.Println("Switched to the Login state")
			default:
				log.Panic("Unhandled intent")
				return
			}
			continue
		}

		// Login Success
		if p.state == StateLogin && packetID == 2 && !clientToServer {
			p.state = StatePlay
			log.Println("Login success, switched to the Play state")
		}

		// Encryption Request
		if p.state == StateLogin && packetID == 1 && !clientToServer {
			encryptionResponse, err := p.handleEncryptionRequest(packetReader)
			if err != nil {
				log.Panic(err)
			}

			// Respond with an encryption response of our own, this way we never tell the client that encryption is enabled.
			// This makes it so that we only have to deal with decrypting and encrypting from and to the server respectively
			// while communication with the client stays unencrypted.
			if _, err := src.Write(encryptionResponse); err != nil {
				if p.errorChecker(err) {
					return
				}
			}

			// Initialise encryption
			block, err := aes.NewCipher(p.sharedSecret)
			if err != nil {
				log.Panic(err)
			}

			p.serverDecrypt = newCFB8Decrypter(block, p.sharedSecret)
			p.serverEncrypt = newCFB8Encrypter(block, p.sharedSecret)

			p.serverReader = &cipher.StreamReader{S: p.serverDecrypt, R: src}
			p.serverWriter = &cipher.StreamWriter{S: p.serverEncrypt, W: src}
			log.Println("Enabled encryption")
			continue
		}

		// Plugin message
		if p.state == StatePlay && packetID == 0x3F && !clientToServer {
			channel, err := readPrefixedBytes(packetReader)
			if err != nil {
				log.Panic(err)
			}
			data, err := readPrefixedBytes(packetReader)
			if err != nil {
				if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
					log.Panic(err)
				}
			}
			if string(channel) == "MC|Brand" && strings.Contains(string(data), "Hypixel") {
				p.isHypixel = true
				continue
			}
		}

		// Serverbound chat message
		if p.state == StatePlay && packetID == 0x01 && clientToServer && p.isHypixel {
			messageBytes, err := readPrefixedBytes(packetReader)
			if err != nil {
				log.Panic(err)
			}
			message := string(messageBytes)
			if strings.HasPrefix(message, "/sc") {
				if hypixel == nil {
					err = p.writeChatMessageToClient("§bGoMCProxy StatCheck: §cHypixel API features have been disabled", ChatTypeChat, src)
					if err != nil {
						log.Panic(err)
					}
					continue
				}
				messageSplit := strings.Split(message, " ")
				if len(messageSplit) != 2 && len(messageSplit) != 3 {
					err = p.writeChatMessageToClient("§bGoMCProxy StatCheck: §cInvalid amount of arguments", ChatTypeChat, src)
					if err != nil {
						log.Panic(err)
					}
					continue
				}

				var bedwarsType BedwarsType
				var playerNameIndex int
				if len(messageSplit) == 3 {
					var ok bool
					bedwarsType, ok = GetBedwarsType(strings.ToLower(messageSplit[1]))
					if !ok {
						err = p.writeChatMessageToClient("§bGoMCProxy StatCheck: §cInvalid bedwars type", ChatTypeChat, src)
						if err != nil {
							if p.errorChecker(err) {
								return
							}
						}
						continue
					}
					playerNameIndex = 2
				} else {
					if p.bedwarsType != nil {
						bedwarsType = *p.bedwarsType
					} else {
						err = p.writeChatMessageToClient("§bGoMCProxy StatCheck: §cInvalid amount of arguments", ChatTypeChat, src)
						if err != nil {
							log.Panic(err)
						}
						continue
					}
					playerNameIndex = 1
				}

				apiProfile, err := getPlayerProfile(messageSplit[playerNameIndex])
				if err != nil {
					err = p.writeChatMessageToClient("§bGoMCProxy StatCheck: §cInvalid player", ChatTypeChat, src)
					if err != nil {
						if p.errorChecker(err) {
							return
						}
					}
					continue
				}
				playerName := apiProfile.Name
				playerUuid := apiProfile.Id

				bedwarsStats, err := hypixel.getBedwarsStats(playerUuid, bedwarsType)
				if err != nil {
					err = p.writeChatMessageToClient("§bGoMCProxy StatCheck: §cAn error occurred while fetching the bedwars stats", ChatTypeChat, src)
					if err != nil {
						if p.errorChecker(err) {
							return
						}
					}
					continue
				}

				statsMessage := "§6§l" + capitaliseFirst(string(bedwarsType)) + " Bedwars Stats for §b§l[" + fmt.Sprint(bedwarsStats.Stars) + "✫] " + playerName + "§r\n" +
					"§aKills: §f" + fmt.Sprint(bedwarsStats.Kills) + "           §cDeaths: §f" + fmt.Sprint(bedwarsStats.Deaths) + "            §aK§f/§cD: §f" + fmt.Sprint(bedwarsStats.KD) + "\n" +
					"§5Final §2Kills: §f" + fmt.Sprint(bedwarsStats.FinalKills) + "   §5Final §4Deaths: §f" + fmt.Sprint(bedwarsStats.FinalDeaths) + "   §5Final §2K§f/§4D: §f" + fmt.Sprint(bedwarsStats.FinalKD) + "\n" +
					"§aWins: §f" + fmt.Sprint(bedwarsStats.Wins) + "         §cLosses: §f" + fmt.Sprint(bedwarsStats.Losses) + "                §aW§f/§cL: §f" + fmt.Sprint(bedwarsStats.WL) + "\n" +
					"§bWinstreak: §f" + fmt.Sprint(bedwarsStats.Winstreak) + "   §3Beds Broken: §f" + fmt.Sprint(bedwarsStats.BedsBroken)

				err = p.writeChatMessageToClient(statsMessage, ChatTypeChat, src)
				if err != nil {
					if p.errorChecker(err) {
						return
					}
				}
				continue
			}
		}

		// Clientbound server message
		if p.state == StatePlay && packetID == 0x02 && !clientToServer && p.isHypixel {
			messageBytes, err := readPrefixedBytes(packetReader)
			if err != nil {
				log.Panic(err)
			}
			message := string(messageBytes)

			if strings.HasPrefix(message, "{\"text\":\"{\\\"server\\\"") {
				chatMessage := ChatMessageData{}
				err = json.Unmarshal([]byte(message), &chatMessage)
				if err != nil {
					log.Panic(err)
				}

				locraw := Locraw{}
				err = json.Unmarshal([]byte(chatMessage.Text), &locraw)
				if err != nil {
					continue
				}

				if locraw.GameType == "BEDWARS" && locraw.Mode != "" {
					bedwarsType, ok := GetBedwarsType(locraw.Mode)
					if ok {
						p.bedwarsType = &bedwarsType
					}
				} else {
					p.bedwarsType = nil
				}
				continue
			}
		}

		// Respawn
		if p.state == StatePlay && packetID == 0x07 && !clientToServer && p.isHypixel {
			dimension := make([]byte, 4)
			_, err := io.ReadFull(packetReader, dimension)
			if err != nil {
				log.Panic(err)
			}

			if int32(binary.BigEndian.Uint32(dimension)) == -1 {
				var packetBody bytes.Buffer

				// Packet ID
				if err := writeVarInt(&packetBody, 0x01); err != nil {
					log.Panic(err)
				}

				locraw := "/locraw"
				// Name length + Name
				if err := writeVarInt(&packetBody, len(locraw)); err != nil {
					log.Panic(err)
				}
				packetBody.Write([]byte(locraw))

				reconstructedPacket, err := p.reconstructPacket(packetBody.Bytes())
				if err != nil {
					log.Panic(err)
				}

				p.writeToSrc(reconstructedPacket, src, clientToServer)
			}
		}

		reconstructedPacket, err := p.reconstructPacket(packetData)
		if err != nil {
			log.Panic(err)
		}

		err = p.writeToDst(reconstructedPacket, dst, clientToServer)
		if err != nil {
			if p.errorChecker(err) {
				return
			}
		}

		// Set Compression
		if p.state == StateLogin && packetID == 3 && !clientToServer {
			localThreshold, _, err := readVarInt(packetReader)
			if err != nil {
				log.Panic("Read error:", err)
			}
			p.threshold = localThreshold
		}
	}
}

// Returns:
// bool: should return
func (p *Proxy) errorChecker(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, syscall.EPIPE) {
		if p.shouldExit {
			p.exited <- struct{}{}
			return true
		}
		p.shouldExit = true
		return true
	}
	log.Panic(err)
	return false
}

type ChatMessageData struct {
	Extra []string `json:"extra"`
	Text  string   `json:"text"`
}

// Creates a **Clientbound** chat message packet
func createChatMessagePacket(text string, chatType ChatType) ([]byte, error) {
	var packetBody bytes.Buffer

	// Packet ID
	if err := writeVarInt(&packetBody, 0x02); err != nil {
		return nil, err
	}

	var jsonData []byte
	var err error
	switch chatType {
	case ChatTypeChat:
		jsonData, err = json.Marshal(ChatMessageData{[]string{text}, ""})
	default:
		log.Panic(errors.New("Not implemented"))
	}
	if err != nil {
		log.Panic(err)
	}

	// JSON data length + JSON data
	if err := writeVarInt(&packetBody, len(jsonData)); err != nil {
		return nil, err
	}
	packetBody.Write(jsonData)

	// Position
	packetBody.Write([]byte{byte(chatType)})

	return packetBody.Bytes(), nil
}

func (p *Proxy) writeChatMessageToClient(text string, chatType ChatType, w io.Writer) error {
	chatMessagePacket, err := createChatMessagePacket(text, chatType)
	if err != nil {
		return err
	}

	reconstructedPacket, err := p.reconstructPacket(chatMessagePacket)
	if err != nil {
		return err
	}

	_, err = w.Write(reconstructedPacket)
	if err != nil {
		return err
	}
	return nil
}

func (p *Proxy) writeToDst(reconstructedPacket []byte, w io.Writer, clientToServer bool) error {
	if p.serverWriter != nil && clientToServer {
		w = p.serverWriter
	}
	if _, err := w.Write(reconstructedPacket); err != nil {
		return err
	}
	return nil
}

func (p *Proxy) writeToSrc(reconstructedPacket []byte, w io.Writer, clientToServer bool) error {
	if p.serverWriter != nil && !clientToServer {
		w = p.serverWriter
	}
	if _, err := w.Write(reconstructedPacket); err != nil {
		return err
	}
	return nil
}

type JoinRequest struct {
	AccessToken     string `json:"accessToken"`
	SelectedProfile string `json:"selectedProfile"` // UUID without dashes
	ServerID        string `json:"serverId"`
}

func (p *Proxy) handleEncryptionRequest(packetReader *bytes.Reader) ([]byte, error) {
	serverIDBytes, err := readPrefixedBytes(packetReader)
	if err != nil {
		return nil, err
	}
	serverID := string(serverIDBytes)

	pubKeyBytes, err := readPrefixedBytes(packetReader)
	if err != nil {
		return nil, err
	}

	parsedServerPubKey, err := x509.ParsePKIXPublicKey(pubKeyBytes)
	if err != nil {
		return nil, err
	}
	p.serverPublicKey = parsedServerPubKey.(*rsa.PublicKey)

	encodedServerPubKey, err := x509.MarshalPKIXPublicKey(p.serverPublicKey)
	if err != nil {
		return nil, err
	}

	verifyToken, err := readPrefixedBytes(packetReader)
	if err != nil {
		return nil, err
	}

	p.sharedSecret = make([]byte, 16)
	rand.Read(p.sharedSecret)

	digest := minecraftDigest(serverID, p.sharedSecret, encodedServerPubKey)

	uuidWithoutDashes := strings.ReplaceAll(p.uuid, "-", "")
	reqBody, err := json.Marshal(JoinRequest{p.accessToken, uuidWithoutDashes, digest})
	if err != nil {
		return nil, err
	}

	resp, err := http.Post("https://sessionserver.mojang.com/session/minecraft/join", "application/json", bytes.NewReader(reqBody))
	if err != nil || resp.StatusCode != 204 {
		return nil, errors.New("Invalid response from Mojang. Check your access token and UUID")
	}

	return p.createEncryptionResponse(verifyToken)
}

func (p *Proxy) createEncryptionResponse(verifyToken []byte) ([]byte, error) {
	encryptedSharedSecretForServer, err := rsa.EncryptPKCS1v15(rand.Reader, p.serverPublicKey, p.sharedSecret)
	if err != nil {
		return nil, err
	}
	encryptedVerifyTokenForServer, err := rsa.EncryptPKCS1v15(rand.Reader, p.serverPublicKey, verifyToken)
	if err != nil {
		return nil, err
	}

	var reconstructedPacket bytes.Buffer
	var packetBody bytes.Buffer

	// Packet ID
	if err := writeVarInt(&packetBody, 0x01); err != nil {
		return nil, err
	}

	// Shared Secret Length + Shared Secret Key (encrypted with server's pub key)
	if err := writeVarInt(&packetBody, len(encryptedSharedSecretForServer)); err != nil {
		return nil, err
	}
	packetBody.Write(encryptedSharedSecretForServer)

	// Verify Token Length + Verify Token (encrypted with server's pub key)
	if err := writeVarInt(&packetBody, len(encryptedVerifyTokenForServer)); err != nil {
		return nil, err
	}
	packetBody.Write(encryptedVerifyTokenForServer)

	// Turn into a full packet
	if err := writeVarInt(&reconstructedPacket, packetBody.Len()); err != nil {
		return nil, err
	}
	reconstructedPacket.Write(packetBody.Bytes())

	return reconstructedPacket.Bytes(), nil
}

func minecraftDigest(serverID string, sharedSecret, pubKey []byte) string {
	h := sha1.New()
	h.Write([]byte(serverID))
	h.Write(sharedSecret)
	h.Write(pubKey)
	sum := h.Sum(nil)

	digest := new(big.Int).SetBytes(sum)

	// Check bit 159 (the MSB of a 160-bit SHA-1 hash)
	if digest.Bit(159) == 1 {
		// Two's complement for negative representation
		max := new(big.Int).Lsh(big.NewInt(1), 160) // 2^160
		digest.Sub(digest, max)
	}

	return digest.Text(16)
}

func (p *Proxy) reconstructPacket(packet []byte) ([]byte, error) {
	var reconstructedPacket bytes.Buffer
	var compressedPacket bytes.Buffer

	// Compression enabled
	if p.threshold != -1 {
		if len(packet) >= p.threshold {
			var compressBuf bytes.Buffer
			zWriter := zlib.NewWriter(&compressBuf)

			// Compress Packet ID + Data
			if _, err := zWriter.Write(packet); err != nil {
				return nil, err
			}
			zWriter.Close()

			// Write data length (Length of uncompressed Packet ID + data)
			if err := writeVarInt(&compressedPacket, len(packet)); err != nil {
				return nil, err
			}

			// Write compressed packet ID + data
			compressedPacket.Write(compressBuf.Bytes())
		} else {
			// Write data length (Uncompressed so 0)
			if err := writeVarInt(&compressedPacket, 0); err != nil {
				return nil, err
			}

			// Write uncompressed packet ID + data
			compressedPacket.Write(packet)
		}

		// Packet length (length of data length + (compressed packet ID + data))
		if err := writeVarInt(&reconstructedPacket, compressedPacket.Len()); err != nil {
			return nil, err
		}

		// Write the other fields into the reconstructed packet (data length + (compressed packet ID + data))
		reconstructedPacket.Write(compressedPacket.Bytes())
		// Compression disabled
	} else {
		// Packet length (packet ID + data)
		if err := writeVarInt(&reconstructedPacket, len(packet)); err != nil {
			return nil, err
		}

		// Write the other fields into the reconstructed packet (packet ID + data)
		reconstructedPacket.Write(packet)
	}

	return reconstructedPacket.Bytes(), nil
}

// Returns:
// int: packet length
// byte[]: data (packet ID + data)
func (p *Proxy) readPacket(r io.Reader) (int, []byte, error) {
	// Packet Length
	packetLength, _, err := readVarInt(r)
	if err != nil {
		return 0, nil, err
	}
	if packetLength == 0 {
		return 0, nil, nil
	}

	dataLength := -1
	var data []byte

	// Compression enabled
	if p.threshold != -1 {
		var bytesRead int
		dataLength, bytesRead, err = readVarInt(r)
		if err != nil {
			return 0, nil, err
		}

		payloadLength := packetLength - bytesRead
		payload := make([]byte, payloadLength)
		if _, err = io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}

		if dataLength > 0 {
			// Packet is compressed
			// Packet ID + Data
			zr, err := zlib.NewReader(bytes.NewReader(payload))
			if err != nil {
				return 0, nil, err
			}
			defer zr.Close()

			data = make([]byte, dataLength)
			_, err = io.ReadFull(zr, data)
			if err != nil {
				return 0, nil, err
			}

		} else {
			// Packet is not compressed
			data = payload
		}
		// Compression disabled
	} else {
		data = make([]byte, packetLength)
		_, err = io.ReadFull(r, data)
		if err != nil {
			return 0, nil, err
		}
	}
	return packetLength, data, err
}

func readPrefixedBytes(r io.Reader) ([]byte, error) {
	bytesLength, _, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	bytesBuf := make([]byte, bytesLength)
	_, err = io.ReadFull(r, bytesBuf)
	return bytesBuf, err
}

func readVarInt(r io.Reader) (int, int, error) {
	var num int
	var shift uint
	var bytesRead int
	for {
		var b [1]byte
		if _, err := r.Read(b[:]); err != nil {
			return 0, 0, err
		}
		bytesRead++
		num |= int(b[0]&0x7F) << shift
		if (b[0] & 0x80) == 0 {
			break
		}
		shift += 7
		if shift > 35 {
			return 0, 0, errors.New("VarInt too big")
		}
	}
	return num, bytesRead, nil
}

func writeVarInt(w io.Writer, value int) error {
	for {
		temp := byte(value & 0x7F)
		value >>= 7
		if value != 0 {
			temp |= 0x80
		}
		if _, err := w.Write([]byte{temp}); err != nil {
			return err
		}
		if value == 0 {
			break
		}
	}
	return nil
}
