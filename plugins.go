package plugins

import (
	"context"
	"errors"
	"io"
	"log"

	plugin "github.com/hashicorp/go-plugin"
	driver "github.com/virtmonitor/driver"
	proto "github.com/virtmonitor/plugins/proto"
	"google.golang.org/grpc"
)

//Handshake Plugin handshake
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "virtmon",
	MagicCookieValue: "foobar",
}

//PluginMap Map of exposable interfaces
var PluginMap = map[string]plugin.Plugin{
	"driver_grpc": &DriverGrpcPlugin{},
}

//DriverGrpcPlugin Plugin struct
type DriverGrpcPlugin struct {
	plugin.Plugin
	Impl driver.Driver
}

//GRPCServer Registers GRPC Server
func (p *DriverGrpcPlugin) GRPCServer(broken *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterDriverServer(s, &DriverServer{Impl: p.Impl})
	return nil
}

//GRPCClient Creates GRPC Client
func (p *DriverGrpcPlugin) GRPCClient(ctx context.Context, broken *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &DriverClient{client: proto.NewDriverClient(c)}, nil
}

//DriverClient Driver client interface
type DriverClient struct {
	client proto.DriverClient
}

//Collect Collects domain statistics from underlying plugin interface
func (m *DriverClient) Collect(cpu, disk, network bool) (map[driver.DomainID]*driver.Domain, error) {
	resp, err := m.client.Collect(context.Background(), &proto.CollectRequest{
		Cpu:     cpu,
		Disk:    disk,
		Network: network,
	})

	if err != nil {
		return nil, err
	}

	var domains []*proto.Domain

	for {
		domain, err := resp.Recv()

		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		domains = append(domains, domain)
	}

	x := make(map[driver.DomainID]*driver.Domain)
	return x, nil

	//return domains, nil
}

func (m *DriverClient) Name() driver.DomainHypervisor {
	resp, err := m.client.Name(context.Background(), &proto.Empty{})
	if err != nil {
		log.Println("Name error:", err)
		return driver.DomainHypervisor("")
	}
	return driver.DomainHypervisor(resp.Name)
}

func (m *DriverClient) Detect() bool {
	resp, err := m.client.Detect(context.Background(), &proto.Empty{})
	if err != nil {
		log.Println("Detect error:", err)
		return false
	}
	return resp.IsHypervisor
}

//Close Signals the driver to cleanup/close
func (m *DriverClient) Close() {}

//DriverServer Server struct
type DriverServer struct {
	Impl driver.Driver
}

//Collect Collect domain statistics from Impl and stream the results back
func (m *DriverServer) Collect(req *proto.CollectRequest, res proto.Driver_CollectServer) error {
	domains, err := m.Impl.Collect(req.Cpu, req.Disk, req.Network)

	if err != nil {
		return err
	}

	for _, domain := range domains {

		d, ok := interface{}(domain).(*proto.Domain)
		if !ok {
			return errors.New("Could not cast *driver.Domain => interface => *proto.Domain")
		}

		if err := res.Send(d); err != nil {
			return err
		}

	}

	return nil
}

func (m *DriverServer) Name(ctx context.Context, req *proto.Empty) (*proto.NameResponse, error) {
	log.Printf("Got name request -> %s\r\n", m.Impl.Name())
	return &proto.NameResponse{Name: string(m.Impl.Name())}, nil
}

func (m *DriverServer) Detect(ctx context.Context, req *proto.Empty) (*proto.DetectResponse, error) {
	log.Printf("Got detect request -> %v\r\n", m.Impl.Detect())
	return &proto.DetectResponse{IsHypervisor: m.Impl.Detect()}, nil
}

func (m *DriverServer) Close(ctx context.Context, req *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}
