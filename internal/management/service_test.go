package management

import "testing"

func TestServiceTypeIsValid(test *testing.T) {
	testCases := []struct {
		name        string
		serviceType ServiceType
		wantValid   bool
	}{
		{name: "OCR", serviceType: ServiceTypeOCR, wantValid: true},
		{name: "empty type", serviceType: "", wantValid: false},
		{name: "unsupported type", serviceType: "unknown", wantValid: false},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			if actual := testCase.serviceType.IsValid(); actual != testCase.wantValid {
				test.Errorf("IsValid() = %t, want %t", actual, testCase.wantValid)
			}
		})
	}
}
