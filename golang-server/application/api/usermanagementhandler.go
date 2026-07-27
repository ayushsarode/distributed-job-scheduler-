package api

import (
	"context"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (g *grpcApi) RegisterUser(ctx context.Context, req *connect.Request[pb.RegisterUserRequest]) (*connect.Response[emptypb.Empty], error) {
	userInfo := req.Msg
	user := entity.User{
		FirstName: userInfo.GetFirstName(),
		LastName:  userInfo.GetLastName(),
	}
	err := g.usermanagementservice.RegisterUser(ctx, user)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}
