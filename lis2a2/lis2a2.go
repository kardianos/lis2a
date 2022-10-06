package lis2a2

import "time"

/*
::LIS2A::
The five Latin-1 characters that immediately follow the H (the header ID) define the delimiters to be used
throughout the subsequent records of the message.

The second character in the header record is the field delimiter,
the third character is the repeat delimiter,
the fourth character is the component delimiter, and
the fifth is the escape character.

H|\^&|

Field=|
Repeat=\
Component=^
Escape=&

A field delimiter follows these characters to separate them from subsequent fields.
Another way to view this is that the first field contains H and the second field contains
the repeat, component, and escape delimiters. Using the example delimiters, the first six characters in the
header record would appear as follows: H | \ˆ & |.

&H&		start highlighting text
&N&		normal text (end highlighting)
&F&		imbedded field delimiter character
&S&		imbedded component field delimiter character
&R&		imbedded repeat field delimiter character
&E&		imbedded escape delimiter character
&Xhhhh&	hexadecimal data

H|\^&|||OrgName|123 Easy Street^MyCity, MyState  ZIP||||||P|NCCLS LIS2-A|20221004013849
P|1||||^^||||||||^|-1||||||||
C|1|P|Pat Coded Note 1:^^
C|2|P|Pat Coded Note 2:^^
C|3|P|Pat Coded Note 3:^^
C|4|P|Pat Coded Note 4:^^
C|5|P|NOTE 1^
C|6|P|NOTE 2^
C|7|P|NOTE 3^
*/

// Trigger

type LIS2AName struct{}

// Sequence ID
//
// A non-negative integer in the form of a NM field. The uses of this data type are defined in the chapters defining the segments
// and messages in which it appears.
type SI = string

// Time
//
// Specifies the hour of the day with optional minutes, seconds, fraction of second using a 24-hour clock notation and time
// zone.
type TM = time.Time

type LIS2_A2_R struct {
	LIS2A   LIS2AName `lis2a:",name=LIS2_A2_R,type=tg"`
	Result  R         `lis2a:"1,required,display=Result"`
	Comment []C       `lis2a:"2,required,display=Comment"`
}
type LIS2_A2_O struct {
	LIS2A   LIS2AName `lis2a:",name=LIS2_A2_O,type=tg"`
	Order   O         `lis2a:"1,required,display=Order"`
	Comment []C       `lis2a:"2,required,display=Comment"`
	Result  []R       `lis2a:"3,required,display=Result List"`
}

type LIS2_A2_P struct {
	LIS2A   LIS2AName   `lis2a:",name=LIS2_A2_P,type=tg"`
	P       P           `lis2a:"1,required,display=Patient"`
	Comment []C         `lis2a:",required,display=Comment"`
	Order   []LIS2_A2_O `lis2a:",required,display=Order List"`
}

type LIS2_A2 struct {
	LIS2A        LIS2AName   `lis2a:",name=LIS2_A2,type=t"`
	Header       H           `lis2a:"1,required,display=Message Header"`
	Manufacturer *M          `lis2a:"2,display=Manufacturer Information"`
	Patient      []LIS2_A2_P `lis2a:",required,display=Patient Group"`
	Last         L           `lis2a:"4,required,display=Last"`
}

// Segment

// Header
type H struct {
	LIS2A              LIS2AName `lis2a:",name=H,type=s"`
	FieldSeparator     ST        `lis2a:"1,noescape,fieldsep,omit,required,len=1,display=Field Separator"`
	EncodingCharacters ST        `lis2a:"2,noescape,fieldchars,required,len=3,display=Encoding Characters"`
	MessageControlID   ST        `lis2a:"3,required,len=6,display=Message Control ID"`
	AccessPassword     ST        `lis2a:"4,required,len=60,display=Access Password"`
	SenderID           ST        `lis2a:"5,required,len=60,display=Sender ID"`
	SenderAddress      Address   `lis2a:"6,required,len=600,display=Sender Address"`
	ReceiverID         ST        `lis2a:"10,len=1,display=Receiver ID to verify who this is intended for."`
	ProcessingID       ST        `lis2a:"12,required,len=1,display=How this message shoule be processed: P=Production|T=Training|D=Debug|Q=QA"`
	MessageVersion     ID        `lis2a:"13,required,len=100,display=Message Structure"`
	MessageDateTime    TS        `lis2a:"14,len=26,format=YMDHMS,display=This field contains the date and time that the message was generated."`
}

func (h H) MessageStructureID() string {
	return string(h.MessageVersion)
}

// Patient - 7
type P struct {
	LIS2A        LIS2AName `lis2a:",name=P,type=s"`
	SetID        SI        `lis2a:"1,seq,required,len=4,display=Set ID - P"`
	PracticeID   ID        `lis2a:"2,len=100,display=Practice assigned patient ID."`
	LabID        ID        `lis2a:"3,len=100,display=Laboratory assigned patient ID."`
	GlobalID     ID        `lis2a:"4,len=100,display=Global patient ID such as SSN."`
	PatientName  Name      `lis2a:"5,len=1000,display=Patient Name."`
	MotherMaiden ST        `lis2a:"6,len=1000,display=Patient's Mother's Maiden Name."`
	DateOfBirth  TS        `lis2a:"7,len=8,format=YMD,display=Patient's Date of Birth."`
	Sex          ST        `lis2a:"8,len=1,display=One of: M|F|U."`
	// TODO: Fill out up to 7.35.
}

// Test Order Record
type O struct {
	LIS2A        LIS2AName  `lis2a:",name=O,type=s"`
	SetID        SI         `lis2a:"1,seq,required,len=4,display=Set ID - O"`
	ID           SpecimenID `lis2a:"2,required,len=100,display=Specimen ID"`
	InstrumentID SpecimenID `lis2a:"3,required,len=100,display=Instrument Specimen ID"`
	GlobalID     GlobalID   `lis2a:"4,required,len=100,display=Global Specimen ID"`
	Priority     []ST       `lis2a:"5,required,len=10,display=Test priority: S=STAT|A=ASAP|R=Routine|C=Callback|P=Preoperative"`
	Action       ST         `lis2a:"11,required,len=1,display=What action to take with the order: C=Cancel|A=Add|N=New|P=Pending|L=Reserved|X=Already in Process|Q=Treat as QA"`
	ReportType   ST         `lis2a:"25,required,len=1,display=Report Type: O=Order|C=Corrected|P=Preliminary|F=Final|X=Cancelled|I=Pending|Y=No order for test|Z=No record of patient|Q=Response to query"`
}

// Result Record
type R struct {
	LIS2A LIS2AName `lis2a:",name=R,type=s"`
	SetID SI        `lis2a:"1,seq,required,len=4,display=Set ID - R"`
	ID    UID       `lis2a:"2,required,len=1000,display=Universal Test ID"`
}

// Comment Record
type C struct {
	LIS2A LIS2AName `lis2a:",name=C,type=s"`
	SetID SI        `lis2a:"1,seq,required,len=4,display=Set ID - C"`
}

// Request Information Record
type Q struct {
	LIS2A LIS2AName `lis2a:",name=Q,type=s"`
	SetID SI        `lis2a:"1,seq,required,len=4,display=Set ID - Q"`
}

// Scientific Record
type S struct {
	LIS2A LIS2AName `lis2a:",name=S,type=s"`
	SetID SI        `lis2a:"1,seq,required,len=4,display=Set ID - S"`
}

// Manufacturer Information Record
type M struct {
	LIS2A LIS2AName `lis2a:",name=M,type=s"`
	SetID SI        `lis2a:"1,seq,required,len=4,display=Set ID - M"`
}

// Last Record
type L struct {
	LIS2A LIS2AName `lis2a:",name=L,type=s"`
	SetID SI        `lis2a:"1,seq,required,len=4,display=Set ID - L"`
}

// Datatype

// Time Stamp - 5.6.2
//
// Specifies a point in time.
//
// Format: YYYY[MM[DD[HH[MM[SS[.S[S[S[S]]]]]]]]][+/-ZZZZ]^<degree of precision>
type TS = time.Time

// String Data
//
// String data is left justified with trailing blanks optional. Any displayable (printable) ACSII characters (hexadecimal
// values between 20 and 7E, inclusive, or ASCII decimal values between 32 and 126), except the defined escape characters
// and defined delimiter characters.
type ST = string

// Number
type NUM = string

type ID = string

// Universal Test ID - 5.6.1
type UID struct {
	LIS2A     LIS2AName `lis2a:",name=UID,len=1000,type=d"`
	Code      ST        `lis2a:"1,conditional,len=250,display=Code"`
	Name      ST        `lis2a:"2,conditional,len=250,display=Display Name"`
	Type      ST        `lis2a:"3,conditional,len=250,display=Type"`
	LocalCode ST        `lis2a:"4,conditional,len=250,display=Local/Manufacture Code"`
}

// Fixed Measurements and Units - 5.6.4
type Unit struct {
	LIS2A  LIS2AName `lis2a:",name=UID,len=1000,type=d"`
	Amount NUM       `lis2a:"1,conditional,len=250,display=Amount"`
	Unit   ST        `lis2a:"2,conditional,len=250,display=Unit"`
}

// Address - 5.6.5
type Address struct {
	LIS2A   LIS2AName `lis2a:",name=Address,len=1000,type=d"`
	Street  ST        `lis2a:"1,conditional,len=250,display=Street"`
	City    ST        `lis2a:"2,conditional,len=250,display=City"`
	State   ST        `lis2a:"3,conditional,len=250,display=State"`
	ZIP     ST        `lis2a:"4,conditional,len=250,display=ZIP/Postal Code"`
	Country ST        `lis2a:"5,conditional,len=250,display=Country"`
}

// Provider and User Name- 5.6.6
type Name struct {
	LIS2A  LIS2AName `lis2a:",name=Address,len=1000,type=d"`
	Last   ST        `lis2a:"1,conditional,len=250,display=Last"`
	First  ST        `lis2a:"2,conditional,len=250,display=First"`
	Middle ST        `lis2a:"3,conditional,len=250,display=Middle Initial or Name"`
	Suffix ST        `lis2a:"4,conditional,len=250,display=Suffix"`
	Title  ST        `lis2a:"5,conditional,len=250,display=Title"`
}

// Provider and User Name with ID - 5.6.6
type NameID struct {
	LIS2A  LIS2AName `lis2a:",name=Address,len=1000,type=d"`
	ID     ST        `lis2a:"1,conditional,len=250,display=ID"`
	Last   ST        `lis2a:"2,conditional,len=250,display=Last"`
	First  ST        `lis2a:"3,conditional,len=250,display=First"`
	Middle ST        `lis2a:"4,conditional,len=250,display=Middle Initial or Name"`
	Suffix ST        `lis2a:"5,conditional,len=250,display=Suffix"`
	Title  ST        `lis2a:"6,conditional,len=250,display=Title"`
}

// Specimen ID - 8.4.3
type SpecimenID struct {
	LIS2A LIS2AName `lis2a:",name=Address,len=1000,type=d"`
	ID    ST        `lis2a:"1,conditional,len=250,display=ID"`
	Spec1 ST        `lis2a:"2,conditional,len=250,display=Specifier 1"`
	Spec2 ST        `lis2a:"3,conditional,len=250,display=Specifier 2"`
}

type GlobalID struct {
	LIS2A LIS2AName `lis2a:",name=Address,len=1000,type=d"`
	ID1   ST        `lis2a:"1,conditional,len=250,display=ID 1"`
	ID2   ST        `lis2a:"2,conditional,len=250,display=ID 2"`
	ID3   ST        `lis2a:"3,conditional,len=250,display=ID 3"`
	ID4   ST        `lis2a:"4,conditional,len=250,display=ID 4"`
	ID5   ST        `lis2a:"5,conditional,len=250,display=ID 5"`
	ID6   ST        `lis2a:"6,conditional,len=250,display=ID 6"`
	ID7   ST        `lis2a:"7,conditional,len=250,display=ID 7"`
	ID8   ST        `lis2a:"8,conditional,len=250,display=ID 8"`
}
