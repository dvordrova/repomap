package httpapi

import (
	"encoding/json"
	"net/http"

	"example.invalid/fulfilment/internal/checkout"
	"example.invalid/fulfilment/internal/model"
)

func Handler(service *checkout.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders/fulfil", func(writer http.ResponseWriter, request *http.Request) {
		var order model.Order
		if err := json.NewDecoder(request.Body).Decode(&order); err != nil {
			http.Error(writer, "invalid order", http.StatusBadRequest)
			return
		}
		result, err := service.Fulfil(request.Context(), order)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadGateway)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(result)
	})
	return mux
}
