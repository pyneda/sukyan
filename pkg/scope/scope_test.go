package scope

import (
	"testing"
)

func TestStrictScope(t *testing.T) {
	testScope := Scope{}
	testScope.ScopeItems = append(testScope.ScopeItems, DomainScope{
		domain: "test.com",
		scope:  "strict",
	})
	testScope.ScopeItems = append(testScope.ScopeItems, DomainScope{
		domain: "stest.com",
		scope:  "strict",
	})
	testScope.ScopeItems = append(testScope.ScopeItems, DomainScope{
		domain: "xyz.com",
	})
	if !testScope.IsInScope("https://test.com/xyz/2?q=test") {
		t.Error()
	}
	if testScope.IsInScope("https://www.test.com/xyz/2?q=test") {
		t.Error()
	}
	if testScope.IsInScope("https://cdn.test.com/xyz/2?q=test") {
		t.Error()
	}
	if !testScope.IsInScope("https://xyz.com/search#test") {
		t.Error()
	}
	if !testScope.IsInScope("https://stest.com/search#test") {
		t.Error()
	}
	if testScope.IsInScope("https://s.stest.com/search#test") {
		t.Error()
	}
	if testScope.IsInScope("https://xyz.xyz.com") {
		t.Error()
	}
}

func TestSubdomainsScope(t *testing.T) {
	testScope := Scope{}
	testScope.ScopeItems = append(testScope.ScopeItems, DomainScope{
		domain: "test.com",
		scope:  "subdomains",
	})
	testScope.ScopeItems = append(testScope.ScopeItems, DomainScope{
		domain: "stest.com",
		scope:  "subdomains",
	})

	if !testScope.IsInScope("https://test.com/xyz/2?q=test") {
		t.Error()
	}
	if !testScope.IsInScope("https://www.test.com/xyz/2?q=test") {
		t.Error()
	}
	if !testScope.IsInScope("https://static.test.com/xyz/2?q=test") {
		t.Error()
	}
	if !testScope.IsInScope("https://www.static.images.test.com/xyz/2?q=test") {
		t.Error()
	}
	if testScope.IsInScope("https://test.org/") {
		t.Error()
	}
	if !testScope.IsInScope("https://search.stest.com/search#test") {
		t.Error()
	}
	if !testScope.IsInScope("https://www.stest.com/search#test") {
		t.Error()
	}
}

func TestWwwScope(t *testing.T) {
	testScope := Scope{}
	testScope.ScopeItems = append(testScope.ScopeItems, DomainScope{
		domain: "test.com",
		scope:  "www",
	})
	testScope.ScopeItems = append(testScope.ScopeItems, DomainScope{
		domain: "stest.com",
		scope:  "www",
	})

	if !testScope.IsInScope("https://test.com/xyz/2?q=test") {
		t.Error()
	}
	if !testScope.IsInScope("https://www.test.com/xyz/2?q=test") {
		t.Error()
	}
	if testScope.IsInScope("https://static.test.com/xyz/2?q=test") {
		t.Error()
	}
	if testScope.IsInScope("https://www.static.test.com/xyz/2?q=test") {
		t.Error()
	}
	if testScope.IsInScope("https://www.www.test.com/xyz/2?q=test") {
		t.Error()
	}
	if testScope.IsInScope("https://test.org/") {
		t.Error()
	}
	if !testScope.IsInScope("https://stest.com/") {
		t.Error()
	}
	if !testScope.IsInScope("https://www.stest.com/") {
		t.Error()
	}
	if testScope.IsInScope("https://www.www.stest.com/") {
		t.Error()
	}
}

func TestIPScope(t *testing.T) {
	testScope := Scope{}
	testScope.ScopeItems = append(testScope.ScopeItems, DomainScope{
		domain: "55.55.200.5",
		scope:  "strict",
	})
	testScope.ScopeItems = append(testScope.ScopeItems, DomainScope{
		domain: "192.168.1.10",
		scope:  "strict",
	})
	testScope.ScopeItems = append(testScope.ScopeItems, DomainScope{
		domain: "127.0.0.1:8000",
		scope:  "www",
	})

	if !testScope.IsInScope("https://55.55.200.5/xyz/2?q=test") {
		t.Error()
	}
	if !testScope.IsInScope("https://55.55.200.5:8000/xyz/2?q=test") {
		t.Error()
	}
	if testScope.IsInScope("https://50.55.200.55/xyz/2?q=test") {
		t.Error()
	}
	if !testScope.IsInScope("https://192.168.1.10:8000/xyz/2?q=test") {
		t.Error()
	}
	if !testScope.IsInScope("https://127.0.0.1:8000/xyz/2?q=test") {
		t.Error()
	}
	if testScope.IsInScope("https://127.0.0.1:80/") {
		t.Error()
	}
}

func TestCreateScopeItemsFromUrls(t *testing.T) {
	testScope := Scope{}
	urlsToTest := []string{"https://test.com", "https://test2.com"}
	testScope.CreateScopeItemsFromUrls(urlsToTest, "www")
	if !testScope.IsInScope("https://test.com") {
		t.Error()

	}

}

func TestAddScopeItem(t *testing.T) {
	testScope := Scope{}
	testScope.ScopeItems = testScope.AddScopeItem("test.com", "wwww")
	if !testScope.IsInScope("https://test.com") {
		t.Error()
	}
}
