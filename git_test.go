package main

import "testing"

func TestParseADOURL(t *testing.T) {
	cases := []struct {
		url                string
		org, project, repo string
		ok                 bool
	}{
		// 실제 보드에 뜨는 리모트들
		{"git@ssh.dev.azure.com:v3/travelflan-dev/TravelFlan/flanb-delivery", "travelflan-dev", "TravelFlan", "flanb-delivery", true},
		{"https://travelflan-dev.visualstudio.com/Harmony/_git/harmony-base", "travelflan-dev", "Harmony", "harmony-base", true},
		// 기타 ADO 형태
		{"https://dev.azure.com/travelflan-dev/TravelFlan/_git/flanb-delivery", "travelflan-dev", "TravelFlan", "flanb-delivery", true},
		{"https://travelflan-dev@dev.azure.com/travelflan-dev/TravelFlan/_git/flanb-delivery.git", "travelflan-dev", "TravelFlan", "flanb-delivery", true},
		{"https://travelflan-dev.visualstudio.com/DefaultCollection/Harmony/_git/harmony-base", "travelflan-dev", "Harmony", "harmony-base", true},
		{"git@vs-ssh.visualstudio.com:v3/travelflan-dev/Harmony/harmony-base", "travelflan-dev", "Harmony", "harmony-base", true},
		// 공백(인코딩) 프로젝트명
		{"https://dev.azure.com/org/My%20Project/_git/repo", "org", "My Project", "repo", true},
		// 비-ADO → ok=false
		{"https://github.com/kimwoo123/endoscope.git", "", "", "", false},
		{"git@github.com:kimwoo123/endoscope.git", "", "", "", false},
		{"", "", "", "", false},
	}
	for _, c := range cases {
		org, project, repo, ok := parseADOURL(c.url)
		if ok != c.ok || org != c.org || project != c.project || repo != c.repo {
			t.Errorf("parseADOURL(%q) = (%q,%q,%q,%v); want (%q,%q,%q,%v)",
				c.url, org, project, repo, ok, c.org, c.project, c.repo, c.ok)
		}
	}
}
