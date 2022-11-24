package mailchimp

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/go-resty/resty/v2"
)

type Merge struct {
	FirstName string `json:"FNAME"`
}

type Audience struct {
	Email       string `json:"email_address"`
	MergeFields Merge  `json:"merge_fields"`
	Status      string `json:"status_if_new"`
}

type Recipients struct {
	ListId string `json:"list_id"`
}

type Settings struct {
	SubjectLine string `json:"subject_line"`
	Title       string `json:"title"`
	ToName      string `json:"to_name"`
	ReplyTo     string `json:"reply_to"`
	FromName    string `json:"from_name"`
}

type Campaign struct {
	RecipientsField Recipients `json:"recipients"`
	Type            string     `json:"type"`
	SettingsField   Settings   `json:"settings"`
}

type CampaignId struct {
	Id string `json:"id"`
}

type CampaignContent struct {
	Html string `json:"html"`
}

type Sections struct {
	OpenPaymentHostMerge        string `json:"body_content"`
	OpenPaymentHostCommentMerge string `json:"body_comment_content"`
}

type TemplateContent struct {
	Id            int      `json:"id"`
	SectionsField Sections `json:"sections"`
}

type Template struct {
	TemplateField TemplateContent `json:"template"`
}

func AddToAudience(audience Audience, list_id string, hash string, token string) {
	// Create a Resty Client
	client := resty.New()

	// Request goes as JSON content type
	// No need to set auth token, error, if you have client level settings
	resp, err := client.R().
		SetBody(Audience{
			Email:       audience.Email,
			MergeFields: Merge{FirstName: audience.MergeFields.FirstName},
			Status:      audience.Status,
		}).
		SetHeader("Content-Type", "application/json").
		SetAuthToken(token).
		Put("https://us12.api.mailchimp.com/3.0/lists/" + list_id + "/members/" + hash)

	log.Info(log.V{"msg": "Mailchimp, Response after adding to mailchimp list", "response": resp.Body()})
	// Explore response object
	if err != nil {
		// Explore response object
		log.Error(log.V{"msg": "Mailchimp, error adding user to the audience list", "error": err, "response": resp.Body()})
	}
}

func UpdateToAudience(audience Audience, list_id string, hash string, token string) {
	// Create a Resty Client
	client := resty.New()

	// Request goes as JSON content type
	// No need to set auth token, error, if you have client level settings
	resp, err := client.R().
		SetBody(Audience{
			Email:       audience.Email,
			MergeFields: Merge{FirstName: audience.MergeFields.FirstName},
			Status:      audience.Status,
		}).
		SetHeader("Content-Type", "application/json").
		SetAuthToken(token).
		Patch("https://us12.api.mailchimp.com/3.0/lists/" + list_id + "/members/" + hash)

	// Explore response object
	if err != nil {
		// Explore response object
		log.Error(log.V{"msg": "Mailchimp, error updating user to the audience list", "error": err, "response": resp.Body()})
	}
}

func CreateCampaign(campaign Campaign, token string) *CampaignId {
	// Create a Resty Client
	client := resty.New()

	// Request goes as JSON content type
	// No need to set auth token, error, if you have client level settings
	resp, err := client.R().
		SetBody(Campaign{
			RecipientsField: Recipients{ListId: campaign.RecipientsField.ListId},
			Type:            "regular",
			SettingsField: Settings{
				SubjectLine: campaign.SettingsField.SubjectLine,
				Title:       campaign.SettingsField.Title,
				ReplyTo:     campaign.SettingsField.ReplyTo,
				FromName:    campaign.SettingsField.FromName,
				ToName:      campaign.SettingsField.ToName,
			},
		}).
		SetHeader("Content-Type", "application/json").
		SetResult(&CampaignId{}).
		SetAuthToken(token).
		Post("https://us12.api.mailchimp.com/3.0/campaigns")

	if err != nil {
		// Explore response object
		log.Error(log.V{"msg": "Mailchimp, error creating campaign", "error": err, "response": resp.Body()})
	}

	return resp.Result().(*CampaignId)
}

func SetCampaignContent(campaignId *CampaignId, template Template, token string) *CampaignContent {
	// Create a Resty Client
	client := resty.New()

	// Request goes as JSON content type
	// No need to set auth token, error, if you have client level settings
	resp, err := client.R().
		SetBody(Template{TemplateField: TemplateContent{
			Id:            template.TemplateField.Id,
			SectionsField: Sections{OpenPaymentHostMerge: template.TemplateField.SectionsField.OpenPaymentHostMerge, OpenPaymentHostCommentMerge: template.TemplateField.SectionsField.OpenPaymentHostCommentMerge},
		}}).
		SetHeader("Content-Type", "application/json").
		SetResult(&CampaignContent{}).
		SetAuthToken(token).
		Put("https://us12.api.mailchimp.com/3.0/campaigns/" + campaignId.Id + "/content")

	if err != nil {
		// Explore response object
		log.Error(log.V{"msg": "Mailchimp, error setting campaign content", "error": err, "response": resp.Body()})
	}

	return resp.Result().(*CampaignContent)
}

func SendCampaign(campaignId *CampaignId, token string) {
	// Create a Resty Client
	client := resty.New()

	// Request goes as JSON content type
	// No need to set auth token, error, if you have client level settings
	resp, err := client.R().
		EnableTrace().
		SetHeader("Content-Type", "application/json").
		SetAuthToken(token).
		Post("https://us12.api.mailchimp.com/3.0/campaigns/" + campaignId.Id + "/actions/send")

	if err != nil {
		// Explore response object
		log.Error(log.V{"msg": "Mailchimp, error sending campaign", "error": err, "response": resp.Body()})
	}
}

func GetMD5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}
