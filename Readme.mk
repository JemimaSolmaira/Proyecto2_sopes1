INFORME TÉCNICO

Sistema “Blackfriday” desplegado en Kubernetes (Minikube/GKE-compatible) con Zot Registry externo, Kafka, Valkey (KubeVirt) y Grafana
1. Resumen ejecutivo

Este proyecto implementa un sistema distribuido orientado a microservicios que procesa tráfico generado por Locust, expuesto mediante Ingress. Las solicitudes llegan a una API REST en Rust, la cual reenvía la carga a componentes en Go que actúan como cliente gRPC y publicadores hacia Kafka. Un consumidor procesa los mensajes y persiste los datos en Valkey. Finalmente, Grafana visualiza métricas/datos desde Valkey mediante un datasource Redis/Valkey. Todas las imágenes Docker se almacenan y distribuyen desde un registry Zot alojado en una VM fuera del clúster.


El sistema cumple con lo siguiente:

2.1 Locust (Generación de tráfico)

Locust genera tráfico HTTP hacia el Ingress Controller.

Cada request contiene un JSON con estructura obligatoria:

categoria

producto

precio

cantidad_vendida

Su objetivo es simular carga controlada y escalable contra el punto de entrada del sistema (Ingress).

2.2 Deployments Rust (API REST)

Deployment Rust expone una API REST que recibe tráfico desde Locust.

Este componente:

recibe la petición,

valida/transforma el JSON,

reenvía el payload hacia el componente Go (API REST y gRPC Client).

Debe soportar alta carga y escalar con HPA.

Escalamiento obligatorio: 1 a 3 réplicas con umbral CPU > 30%.

2.3 Deployments Go

Se implementan tres despliegues lógicos:

Deployment 1: Go (API REST + gRPC Client)

Recibe solicitudes desde Rust.

Actúa como cliente gRPC hacia el/los servidores gRPC.

Invoca funciones para publicar mensajes (indirectamente vía server/writers) hacia Kafka.

Deployment 2 y 3: Go (gRPC Server + Writers)

Exponen endpoint gRPC.

Procesan llamadas desde el cliente.

Publican mensajes hacia Kafka (Kafka writer).

Pruebas obligatorias con 1 y 2 réplicas para validar escalamiento y balanceo.

2.4 Kafka (Strimzi)

Deployment/cluster de Kafka implementado con Strimzi.

Función:

almacenar mensajes publicados,

distribuirlos al consumidor.

Kafka funciona como capa de desacoplamiento para tolerancia a picos y buffering.

2.5 Deployment Consumidor Kafka

Un deployment consumidor:

consume mensajes desde Kafka,

extrae los campos del JSON,

persiste la información en Valkey.

Es el componente que asegura que el dato termine almacenado en la base (Valkey).

2.6 Deployments Valkey con persistencia y replicación

Valkey se despliega con persistencia garantizada.

Requisito obligatorio:

2 réplicas por defecto

almacenamiento persistente asegurado (disco/volume).

En tu diseño, Valkey corre en VMs administradas por KubeVirt dentro del clúster.

Nota técnica importante: es válido que solo el primary persista en disco y las réplicas mantengan rol de réplica (dependiendo de configuración), siempre que el primary tenga persistencia garantizada y la réplica cumpla rol de redundancia/lectura.

2.7 Deployment Grafana (visualización)

Grafana se despliega en Kubernetes como Deployment + Service + (opcional) Ingress.

Visualiza datos conectándose a Valkey mediante plugin Redis datasource.

Instalación recomendada: Helm, porque simplifica:

almacenamiento persistente,

configuración admin,

ingress,

provisión de datasources/dashboards.

2.8 Zot Registry (VM externa a Kubernetes)

Zot se aloja en una VM de GCP fuera del clúster.

Flujo de imágenes:

se construyen imágenes Docker de cada componente,

se hace push a Zot,

Kubernetes hace pull desde Zot al crear pods (controlado por imagePullPolicy y credenciales si aplica).

Esto asegura independencia del clúster y un flujo tipo “registry enterprise”.

3. Arquitectura del sistema (flujo extremo a extremo)
3.1 Flujo de entrada

Locust envía HTTP al Ingress (ej. http://api.local).

Ingress NGINX enruta al Service del API Rust.

Rust API REST procesa y reenvía al Go Deployment 1.

Go API REST + gRPC Client invoca gRPC Server.

gRPC Server/Writers publica eventos en Kafka.

3.2 Flujo de mensajería y persistencia

Kafka recibe y retiene mensajes.

Kafka Consumer consume los mensajes y guarda en Valkey.

3.3 Observabilidad de datos

Grafana consulta Valkey mediante datasource Redis/Valkey y muestra dashboards.

4. Componentes Kubernetes (tipos de recursos)

Los componentes se implementan con:

Deployment: Rust, Go Client, Go Server, Consumer, Grafana

Service (ClusterIP): exponer internamente cada app

Ingress (NGINX): exponer API y Grafana por hostname

HPA: escalamiento automático para Rust (y opcionalmente otros)

Kafka (Strimzi): CRDs (Kafka, KafkaTopic, etc.)

KubeVirt: VirtualMachine / VirtualMachineInstance para Valkey

ConfigMaps/Secrets:

configuraciones,

credenciales,

datasource de Grafana (provisioning)

5. Persistencia y disponibilidad de Valkey

Para cumplir “persistencia asegurada” se recomienda:

Volumen persistente para el primary (RDB/AOF).

Réplicas conectadas al primary (redundancia y lectura).

Validaciones típicas:

INFO persistence (estado de snapshot o AOF)

INFO replication (role, connected_slaves, master_link_status)

6. Grafana + Valkey (cómo se “obtienen datos”)

Grafana no “lee la BD como SQL”, sino que consulta Valkey por comandos/operaciones que el datasource Redis entiende.

Estrategia típica:

Consumer guarda datos en estructuras consultables (por ejemplo):

Hash por producto/categoría

Sorted sets por ranking de ventas

Series temporales (si usas RedisTimeSeries o claves por timestamp)

Grafana construye paneles con consultas a esas claves.

El datasource se puede “provisionar” por YAML (ConfigMap) para que al levantar Grafana ya exista la conexión.

7. Validación de funcionamiento (pruebas recomendadas para evidencia)
7.1 Verificación de despliegue general

Pods/estado:

kubectl get pods -A

Servicios:

kubectl get svc

Ingress:

kubectl get ingress

7.2 Prueba de entrada por Ingress

Validar que la API responde por hostname:

curl http://api.local/...

7.3 Verificación de pipeline

Durante una prueba Locust, observar logs:

Rust recibe requests

Go client invoca gRPC

gRPC server publica a Kafka

Consumer consume y escribe a Valkey

Valkey contiene claves/datos esperados

7.4 Validación de HPA

Ver HPA:

kubectl get hpa

Forzar carga con Locust y verificar incremento de réplicas.

7.5 Validación Zot

Ver que los pods ya no caen en ImagePullBackOff.

Confirmar que las imágenes están siendo obtenidas desde Zot (eventos del pod).

8. Justificación técnica: por qué esta arquitectura es correcta

Ingress: centraliza entrada y expone por hostnames.

Rust API + HPA: escala el frontend de entrada.

Go + gRPC: comunicación eficiente entre microservicios.

Kafka: desacopla ingreso vs procesamiento; tolera picos y asegura delivery.

Consumer: separación clara entre ingestión y persistencia.

Valkey: persistencia rápida (key-value) y replicación para alta disponibilidad.

Grafana: dashboarding estándar Cloud Native.

Zot: registry privado externo, alineado a entornos reales.

9. Entregables del sistema

Manifiestos Kubernetes (.yaml) para:

deployments + services

ingress

hpa

kafka/strimzi resources

valkey/kubevirt

grafana + datasource provisioning

Evidencia:

capturas de kubectl get pods -A

logs del pipeline

dashboard de Grafana con datasource Valkey conectado

prueba de Locust corriendo hacia Ingress

10. Conclusión

El sistema implementa un flujo completo de generación de carga → ingestión → procesamiento → mensajería → persistencia → visualización, cumpliendo los requisitos obligatorios del documento: uso de Locust con JSON definido, escalamiento HPA en Rust, pipeline Go/gRPC, Kafka, consumidor con persistencia en Valkey replicado, Grafana para dashboards y Zot como registry externo en VM.