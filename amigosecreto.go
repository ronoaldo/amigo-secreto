package amigosecreto

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

var TableName = "GrupoAmigoSecreto"

func Handler(ctx context.Context, req events.APIGatewayProxyRequest) (resp events.APIGatewayProxyResponse, err error) {
	log.Printf("Received: %#v", req)
	if req.QueryStringParameters == nil {
		errorf(&resp, "missing arguments grupo")
		return resp, nil
	}

	grupo := req.QueryStringParameters["grupo"]
	acao := req.QueryStringParameters["acao"]
	quemSou := req.QueryStringParameters["quem-sou"]
	chave := req.QueryStringParameters["chave"]

	// Initialize values
	body := strings.Builder{}
	url := fmt.Sprintf("https://%s", req.RequestContext.DomainName)
	resp.Headers = map[string]string{
		"content-type": "text/html; charset=utf-8",
	}

	log.Printf("Processando: grupo=%s, acao=%s\n", grupo, acao)
	switch acao {
	case "sortear":
		err = Sortear(&body, grupo, url)
	case "gerar-links":
		err = GerarLinks(&body, grupo, url)
	case "ver-amigo":
		err = VerMeuAmigoSecreto(&body, quemSou, grupo, chave)
	default:
		err = Index(&body, grupo)
	}

	if err != nil {
		errorf(&resp, "erro ao %s: %v", acao, err)
		return resp, nil
	}
	resp.Body = body.String()
	return
}

func errorf(resp *events.APIGatewayProxyResponse, msg string, args ...any) {
	resp.Body = fmt.Sprintf(msg, args...)
	resp.StatusCode = 400
	resp.Headers = map[string]string{"content-type": "text/plain"}
}

type AmigoSecreto struct {
	Grupo   string            `dynamodbav:"amigosecreto"`
	Amigos  []string          `dynamodbav:"amigos"`
	Sorteio map[string]string `dynamodbav:"sorteio"`
	Seed    int64             `dynamodbav:"seed"`
}

func Index(w io.Writer, grupo string) error {
	svc := connectar()
	amigosecreto, err := carregar(svc, grupo)
	if err != nil {
		return err
	}

	return show(w, TemplateIndex, map[string]interface{}{
		"amigosecreto": amigosecreto,
		"grupo":        grupo,
	})
}

func VerMeuAmigoSecreto(w io.Writer, quemSou, grupo, chave string) error {
	svc := connectar()
	amigosecreto, err := carregar(svc, grupo)
	if err != nil {
		return err
	}

	// Validar a chave
	chaveEsperada := criaChaveSecreta(quemSou, grupo, amigosecreto.Seed)
	if chaveEsperada != chave {
		log.Printf("chave inválida [quemSou=%v, grupo=%v]: chave esperada: %v, informada: %v", quemSou, grupo, chaveEsperada, chave)
		return fmt.Errorf("chave inválida: %v", chave)
	}

	// Exibe quem é seu amigo secreto!
	meuAmigo := amigosecreto.Sorteio[quemSou]
	return show(w, TemplateAmigo, map[string]interface{}{
		"quemSou": quemSou,
		"amigo":   meuAmigo,
	})
}

type Link struct {
	Amigo string
	Link  string
}

func GerarLinks(w io.Writer, grupo string, url string) error {
	svc := connectar()
	amigosecreto, err := carregar(svc, grupo)
	if err != nil {
		return err
	}
	links := make([]Link, 0)
	// Gera os links para o resultado
	for amigo := range amigosecreto.Sorteio {
		link := linkVerAmigo(amigo, grupo, url, amigosecreto.Seed)
		links = append(links, Link{
			Amigo: amigo,
			Link:  link,
		})
	}
	return show(w, TemplateLinks, map[string]interface{}{
		"links": links,
	})
}

// Sortear o amigo secreto utilizando os nomes dos participantes cadastrados
func Sortear(w io.Writer, grupo string, url string) error {
	// Cria um cliente DynamoDB
	svc := connectar()

	// Resgata o grupo do amigo secreto
	amigosecreto, err := carregar(svc, grupo)
	if err != nil {
		return err
	}
	log.Printf("amigosecreto=%#v", amigosecreto)

	// Necessário ao menos três amigos
	if len(amigosecreto.Amigos) < 3 {
		return fmt.Errorf("você tem poucos amigos, precisa de no mínimo 3: %v", len(amigosecreto.Amigos))
	}

	// Realiza o sorteio reordenando os participantes de modo aleatório
	amigosecreto.Seed = time.Now().Unix()
	sorteio := make([]string, 0, len(amigosecreto.Amigos))
	rng := rand.New(rand.NewSource(amigosecreto.Seed))
	for _, sorteado := range rng.Perm(len(amigosecreto.Amigos)) {
		sorteio = append(sorteio, amigosecreto.Amigos[sorteado])
	}
	log.Printf("sorteio=%v", sorteio)
	// Cada participante tirou o seguinte, e o último tirou o primeiro
	for i := 0; i < len(sorteio)-1; i++ {
		amigo, secreto := sorteio[i], sorteio[i+1]
		amigosecreto.Sorteio[amigo] = secreto
	}
	amigosecreto.Sorteio[sorteio[len(sorteio)-1]] = sorteio[0]
	log.Printf("amigosecreto=%#v", amigosecreto)

	// Gera os links para o resultado
	fmt.Fprintf(w, "Sorteio realizado! <a href=%s/?grupo=%s>Voltar</a>", url, grupo)

	// Atualiza o item na base de dados
	err = salvar(svc, amigosecreto)
	return err
}

func connectar() *dynamodb.DynamoDB {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc := dynamodb.New(sess)
	return svc
}

func carregar(svc *dynamodb.DynamoDB, grupo string) (*AmigoSecreto, error) {
	result, err := svc.GetItem(&dynamodb.GetItemInput{
		TableName: &TableName,
		Key: map[string]*dynamodb.AttributeValue{
			"amigosecreto": {S: aws.String(grupo)},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, errors.New("group not found")
	}
	amigosecreto := AmigoSecreto{}
	amigosecreto.Sorteio = make(map[string]string)
	if err = dynamodbattribute.UnmarshalMap(result.Item, &amigosecreto); err != nil {
		return nil, err
	}
	return &amigosecreto, nil
}

func salvar(svc *dynamodb.DynamoDB, a *AmigoSecreto) error {
	av, err := dynamodbattribute.MarshalMap(a)
	if err != nil {
		return err
	}
	_, err = svc.PutItem(&dynamodb.PutItemInput{
		Item:      av,
		TableName: &TableName,
	})
	return err
}

func criaChaveSecreta(amigo, grupo string, seed int64) string {
	buff := fmt.Sprintf("%s:%d:%s", grupo, seed, amigo)
	sum := sha256.Sum256([]byte(buff))
	return fmt.Sprintf("%x", sum)
}

func linkVerAmigo(amigo, grupo string, baseUrl string, seed int64) string {
	chave := criaChaveSecreta(amigo, grupo, seed)
	e := url.QueryEscape
	return fmt.Sprintf("%s/?acao=ver-amigo&quem-sou=%s&grupo=%s&chave=%v\n", baseUrl, e(amigo), e(grupo), e(chave))
}

func show(body io.Writer, html string, context map[string]interface{}) error {
	tpl := template.Must(template.New("template").Parse(html))
	return tpl.Execute(body, context)
}
